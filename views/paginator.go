package views

import (
	"context"
	"errors"
	"iter"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/oliverhaas/djangogo/orm"
)

// Pagination errors, the counterparts of django.core.paginator's exceptions.
// ErrPageNotAnInteger and ErrEmptyPage are kinds of ErrInvalidPage, so
// errors.Is(err, ErrInvalidPage) matches both, as "except InvalidPage" does.
// The error a Paginator returns carries Django's message as its text.
var (
	// ErrInvalidPage is Django's InvalidPage.
	ErrInvalidPage error = &kindError{msg: "views: invalid page"}
	// ErrPageNotAnInteger is Django's PageNotAnInteger.
	ErrPageNotAnInteger error = &kindError{msg: "views: page is not an integer", parent: ErrInvalidPage}
	// ErrEmptyPage is Django's EmptyPage.
	ErrEmptyPage error = &kindError{msg: "views: empty page", parent: ErrInvalidPage}
)

// Ellipsis is the marker GetElidedPageRange puts where pages are left out,
// Django's Paginator.ELLIPSIS.
const Ellipsis = "…"

// Default arguments of Django's get_elided_page_range, for callers of
// GetElidedPageRange that want Django's defaults.
const (
	DefaultOnEachSide = 3
	DefaultOnEnds     = 2
)

// Keys of Paginator.ErrorMessages and their default texts, as in Django.
const (
	msgInvalidPage = "invalid_page"
	msgMinPage     = "min_page"
	msgNoResults   = "no_results"
)

// defaultPageMessages holds Django's default error messages of a paginator.
var defaultPageMessages = map[string]string{
	msgInvalidPage: "That page number is not an integer",
	msgMinPage:     "That page number is less than 1",
	msgNoResults:   "That page contains no results",
}

// ObjectList is the data a Paginator pages through: anything that can be
// counted and sliced. It stands in for the duck typing of Django's Paginator,
// which accepts a list, a QuerySet or any object with count() or __len__().
// Use SliceList for a slice and QuerySetList for a queryset.
type ObjectList[T any] interface {
	// Count returns the total number of objects.
	Count(ctx context.Context) (int, error)
	// Slice returns at most limit objects starting at offset.
	Slice(ctx context.Context, offset, limit int) ([]T, error)
}

// sliceList adapts a slice to ObjectList.
type sliceList[T any] struct{ items []T }

// SliceList returns an ObjectList over an in-memory slice.
func SliceList[T any](items []T) ObjectList[T] { return sliceList[T]{items: items} }

// Count returns the length of the slice.
func (l sliceList[T]) Count(context.Context) (int, error) { return len(l.items), nil }

// Slice returns the sub-slice, clamped to the bounds of the list.
func (l sliceList[T]) Slice(_ context.Context, offset, limit int) ([]T, error) {
	start := min(max(offset, 0), len(l.items))
	end := min(start+max(limit, 0), len(l.items))
	return l.items[start:end], nil
}

// querySetList adapts a queryset to ObjectList.
type querySetList[T any] struct{ qs *orm.QuerySet[T] }

// QuerySetList returns an ObjectList over a queryset. Counting runs one COUNT
// query and each slice one LIMIT/OFFSET query. Order the queryset, otherwise
// the database may return rows in a different order on each page.
func QuerySetList[T any](qs *orm.QuerySet[T]) ObjectList[T] { return querySetList[T]{qs: qs} }

// Count runs COUNT(*) for the queryset.
func (l querySetList[T]) Count(ctx context.Context) (int, error) {
	n, err := l.qs.Count(ctx)
	return int(n), err
}

// Slice loads one window of rows with LIMIT and OFFSET.
func (l querySetList[T]) Slice(ctx context.Context, offset, limit int) ([]T, error) {
	return l.qs.Offset(offset).Limit(limit).All(ctx)
}

// Paginator splits an ObjectList into pages, the counterpart of
// django.core.paginator.Paginator. Create it with NewPaginator. Mapping from
// Django:
//
//	Paginator(object_list, per_page, orphans=0, allow_empty_first_page=True,
//	          error_messages=None)    NewPaginator(list, perPage, options...)
//	paginator.count                   Count(ctx)
//	paginator.num_pages               NumPages(ctx)
//	paginator.page_range              PageRange(ctx), a []int
//	paginator.page(number)            Page(ctx, number)
//	paginator.get_page(number)        GetPage(ctx, number)
//	paginator.validate_number(n)      ValidateNumber(ctx, n)
//	paginator.get_elided_page_range   GetElidedPageRange(ctx, number, onEachSide, onEnds)
//	iter(paginator)                   Pages(ctx)
//	Paginator.ELLIPSIS                Ellipsis
//
// Methods that may reach the database take a context and return an error.
// The count is computed once and cached, so a paginator over a queryset runs
// exactly one COUNT query plus one LIMIT/OFFSET query per page. A Paginator is
// safe for concurrent use; create one per request, because the cached count
// goes stale.
type Paginator[T any] struct {
	// ObjectList is the data to page through.
	ObjectList ObjectList[T]
	// PerPage is the maximum number of items on a page, not counting orphans.
	PerPage int
	// Orphans is the number of items the last page may hold on top of PerPage
	// to avoid a last page with very few items.
	Orphans int
	// AllowEmptyFirstPage decides whether page 1 is valid for an empty list.
	AllowEmptyFirstPage bool
	// ErrorMessages overrides Django's messages by the keys "invalid_page",
	// "min_page" and "no_results". Missing keys keep the default.
	ErrorMessages map[string]string
	// TemplateObject converts an item for the template view of a page (see
	// Page.TemplateValue). ListView sets it to wrap model instances. Nil
	// passes items through unchanged.
	TemplateObject func(T) any

	mu      sync.Mutex
	counted bool
	count   int
}

// PaginatorOption configures a Paginator in NewPaginator.
type PaginatorOption func(*paginatorConfig)

// paginatorConfig collects the optional arguments of NewPaginator.
type paginatorConfig struct {
	orphans             int
	allowEmptyFirstPage bool
	errorMessages       map[string]string
}

// WithOrphans sets Django's orphans argument.
func WithOrphans(n int) PaginatorOption {
	return func(c *paginatorConfig) { c.orphans = n }
}

// WithAllowEmptyFirstPage sets Django's allow_empty_first_page argument, which
// defaults to true.
func WithAllowEmptyFirstPage(allow bool) PaginatorOption {
	return func(c *paginatorConfig) { c.allowEmptyFirstPage = allow }
}

// WithErrorMessages sets Django's error_messages argument.
func WithErrorMessages(messages map[string]string) PaginatorOption {
	return func(c *paginatorConfig) { c.errorMessages = messages }
}

// NewPaginator returns a Paginator over list with perPage items per page.
func NewPaginator[T any](list ObjectList[T], perPage int, opts ...PaginatorOption) *Paginator[T] {
	cfg := paginatorConfig{allowEmptyFirstPage: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Paginator[T]{
		ObjectList:          list,
		PerPage:             perPage,
		Orphans:             cfg.orphans,
		AllowEmptyFirstPage: cfg.allowEmptyFirstPage,
		ErrorMessages:       cfg.errorMessages,
	}
}

// message returns the error message for key, preferring ErrorMessages.
func (p *Paginator[T]) message(key string) string {
	if msg, ok := p.ErrorMessages[key]; ok {
		return msg
	}
	return defaultPageMessages[key]
}

// Count returns the total number of objects across all pages. The first call
// counts the object list; later calls return the cached number.
func (p *Paginator[T]) Count(ctx context.Context) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.counted {
		return p.count, nil
	}
	if p.ObjectList == nil {
		return 0, improperlyConfigured("paginator has no object list")
	}
	n, err := p.ObjectList.Count(ctx)
	if err != nil {
		return 0, err
	}
	p.count, p.counted = n, true
	return n, nil
}

// cachedCount returns the count when it has been computed already.
func (p *Paginator[T]) cachedCount() (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count, p.counted
}

// numPagesFor returns the number of pages for a known count.
func (p *Paginator[T]) numPagesFor(count int) int {
	if count == 0 && !p.AllowEmptyFirstPage {
		return 0
	}
	hits := max(1, count-p.Orphans)
	return int(math.Ceil(float64(hits) / float64(p.PerPage)))
}

// NumPages returns the total number of pages.
func (p *Paginator[T]) NumPages(ctx context.Context) (int, error) {
	if p.PerPage < 1 {
		return 0, improperlyConfigured("paginator per_page must be at least 1, got %d", p.PerPage)
	}
	count, err := p.Count(ctx)
	if err != nil {
		return 0, err
	}
	return p.numPagesFor(count), nil
}

// PageRange returns the 1-based page numbers, for example [1 2 3 4].
func (p *Paginator[T]) PageRange(ctx context.Context) ([]int, error) {
	n, err := p.NumPages(ctx)
	if err != nil {
		return nil, err
	}
	return intRange(1, n), nil
}

// intRange returns the integers from first to last, both included.
func intRange(first, last int) []int {
	if last < first {
		return []int{}
	}
	out := make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		out = append(out, i)
	}
	return out
}

// pageNumber converts a page number of unknown type to an int the way Python's
// int() does for the values a view receives: integers, floats with an integral
// value and decimal strings (surrounding space allowed) are accepted.
func pageNumber(number any) (int, bool) {
	switch n := number.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), n <= math.MaxInt
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), n <= math.MaxInt
	case float32:
		return floatPageNumber(float64(n))
	case float64:
		return floatPageNumber(n)
	case string:
		v, err := strconv.Atoi(strings.TrimSpace(n))
		return v, err == nil
	default:
		return 0, false
	}
}

// floatPageNumber accepts a float only when it holds an integral value.
func floatPageNumber(f float64) (int, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || math.Abs(f) > math.MaxInt32 {
		return 0, false
	}
	return int(f), true
}

// pageError returns an error of the given kind with a paginator message.
func pageError(kind error, msg string) error {
	return &messageError{kind: kind, msg: msg}
}

// ValidateNumber validates a 1-based page number and returns it as an int.
// The number may be any integer type, a float with an integral value or a
// decimal string, which is what a query parameter delivers. It returns an
// ErrPageNotAnInteger for anything else, and an ErrEmptyPage for a number
// below 1 or beyond the last page.
func (p *Paginator[T]) ValidateNumber(ctx context.Context, number any) (int, error) {
	n, ok := pageNumber(number)
	if !ok {
		return 0, pageError(ErrPageNotAnInteger, p.message(msgInvalidPage))
	}
	if n < 1 {
		return 0, pageError(ErrEmptyPage, p.message(msgMinPage))
	}
	numPages, err := p.NumPages(ctx)
	if err != nil {
		return 0, err
	}
	if n > numPages {
		return 0, pageError(ErrEmptyPage, p.message(msgNoResults))
	}
	return n, nil
}

// Page returns the page with the given 1-based number, or an ErrEmptyPage when
// the number is out of range. The items of the page are loaded right away,
// with one LIMIT/OFFSET query for a queryset.
func (p *Paginator[T]) Page(ctx context.Context, number int) (*Page[T], error) {
	number, err := p.ValidateNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	count, err := p.Count(ctx)
	if err != nil {
		return nil, err
	}
	bottom := (number - 1) * p.PerPage
	top := bottom + p.PerPage
	if top+p.Orphans >= count {
		top = count
	}
	items := []T{}
	if top > bottom {
		if items, err = p.ObjectList.Slice(ctx, bottom, top-bottom); err != nil {
			return nil, err
		}
	}
	return &Page[T]{
		ObjectList: items,
		Number:     number,
		Paginator:  p,
		count:      count,
		numPages:   p.numPagesFor(count),
	}, nil
}

// GetPage returns a valid page even for an invalid number, for use with raw
// request input: a number that is not an integer selects the first page and a
// number out of range the last page. It still returns an ErrEmptyPage for an
// empty list when AllowEmptyFirstPage is false, because then no page exists.
func (p *Paginator[T]) GetPage(ctx context.Context, number any) (*Page[T], error) {
	n, err := p.ValidateNumber(ctx, number)
	switch {
	case errors.Is(err, ErrPageNotAnInteger):
		n = 1
	case errors.Is(err, ErrEmptyPage):
		if n, err = p.NumPages(ctx); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}
	return p.Page(ctx, n)
}

// Pages iterates over all pages in order, the counterpart of iterating a
// Django Paginator. Iteration stops after the first error, which is yielded
// with a nil page.
func (p *Paginator[T]) Pages(ctx context.Context) iter.Seq2[*Page[T], error] {
	return func(yield func(*Page[T], error) bool) {
		numPages, err := p.NumPages(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		for n := 1; n <= numPages; n++ {
			page, err := p.Page(ctx, n)
			if !yield(page, err) || err != nil {
				return
			}
		}
	}
}

// GetElidedPageRange returns the page numbers to show in a pagination bar
// around the page number: onEachSide pages on each side of it and onEnds pages
// at both ends, with Ellipsis where pages are left out. The elements are ints
// and the Ellipsis string. Django's defaults are DefaultOnEachSide and
// DefaultOnEnds. For example, with 50 pages, number 8 and the defaults:
//
//	[1 2 … 5 6 7 8 9 10 11 … 49 50]
//
// The number is validated like in ValidateNumber.
func (p *Paginator[T]) GetElidedPageRange(ctx context.Context, number any, onEachSide, onEnds int) ([]any, error) {
	n, err := p.ValidateNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	numPages, err := p.NumPages(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, 2*(onEachSide+onEnds)+3)
	add := func(first, last int) {
		for i := first; i <= last; i++ {
			out = append(out, i)
		}
	}
	if numPages <= (onEachSide+onEnds)*2 {
		add(1, numPages)
		return out, nil
	}
	if n > (1+onEachSide+onEnds)+1 {
		add(1, onEnds)
		out = append(out, Ellipsis)
		add(n-onEachSide, n)
	} else {
		add(1, n)
	}
	if n < (numPages-onEachSide-onEnds)-1 {
		add(n+1, n+onEachSide)
		out = append(out, Ellipsis)
		add(numPages-onEnds+1, numPages)
	} else {
		add(n+1, numPages)
	}
	return out, nil
}

// TemplateValue returns the template view of the paginator, a map with
// Django's attribute names: per_page, orphans, allow_empty_first_page,
// ELLIPSIS, count, num_pages, page_range and get_elided_page_range (called
// with a page number, using Django's default arguments). The count-based
// entries are plain values once the paginator has counted its list, which is
// always the case for the paginator of a Page; before that they are functions
// the template engine calls on first use.
func (p *Paginator[T]) TemplateValue() any {
	out := map[string]any{
		"per_page":               p.PerPage,
		"orphans":                p.Orphans,
		"allow_empty_first_page": p.AllowEmptyFirstPage,
		"ELLIPSIS":               Ellipsis,
		"get_elided_page_range": func(number int) ([]any, error) {
			return p.GetElidedPageRange(context.Background(), number, DefaultOnEachSide, DefaultOnEnds)
		},
	}
	if count, ok := p.cachedCount(); ok && p.PerPage > 0 {
		numPages := p.numPagesFor(count)
		out["count"] = count
		out["num_pages"] = numPages
		out["page_range"] = intRange(1, numPages)
		return out
	}
	// A template has no context to pass, so the lazy entries run detached
	// from the request.
	out["count"] = func() (int, error) { return p.Count(context.Background()) }
	out["num_pages"] = func() (int, error) { return p.NumPages(context.Background()) }
	out["page_range"] = func() ([]int, error) { return p.PageRange(context.Background()) }
	return out
}

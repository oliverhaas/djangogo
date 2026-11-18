package views_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/views"
)

// paginatorParams are the arguments of Django's Paginator constructor in the
// table-driven pagination tests.
type paginatorParams struct {
	objects             []int
	perPage             int
	orphans             int
	allowEmptyFirstPage bool
}

// newIntPaginator builds a paginator over a list of ints from table params.
func newIntPaginator(p paginatorParams) *views.Paginator[int] {
	return views.NewPaginator(views.SliceList(p.objects), p.perPage,
		views.WithOrphans(p.orphans), views.WithAllowEmptyFirstPage(p.allowEmptyFirstPage))
}

// intsUpTo returns [1, 2, ..., n].
func intsUpTo(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

// countContainer is an ObjectList that can only be counted, like the
// CountContainer and LenContainer classes of Django's test.
type countContainer struct {
	n   int
	err error
}

func (c countContainer) Count(context.Context) (int, error) { return c.n, c.err }

func (c countContainer) Slice(context.Context, int, int) ([]int, error) { return nil, nil }

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_paginator
//
// The cases with a string per_page or orphans are left out: both are ints in
// Go.
func TestPaginator(t *testing.T) {
	ctx := context.Background()
	nine, ten, eleven := intsUpTo(9), intsUpTo(10), intsUpTo(11)
	tests := []struct {
		params    paginatorParams
		count     int
		numPages  int
		pageRange []int
	}{
		// Ten items, varying orphans, no empty first page.
		{paginatorParams{ten, 4, 0, false}, 10, 3, []int{1, 2, 3}},
		{paginatorParams{ten, 4, 1, false}, 10, 3, []int{1, 2, 3}},
		{paginatorParams{ten, 4, 2, false}, 10, 2, []int{1, 2}},
		// Ten items, varying orphans, allow empty first page.
		{paginatorParams{ten, 4, 0, true}, 10, 3, []int{1, 2, 3}},
		{paginatorParams{ten, 4, 1, true}, 10, 3, []int{1, 2, 3}},
		{paginatorParams{ten, 4, 2, true}, 10, 2, []int{1, 2}},
		// One item, varying orphans, no empty first page.
		{paginatorParams{[]int{1}, 4, 0, false}, 1, 1, []int{1}},
		{paginatorParams{[]int{1}, 4, 1, false}, 1, 1, []int{1}},
		{paginatorParams{[]int{1}, 4, 2, false}, 1, 1, []int{1}},
		// One item, varying orphans, allow empty first page.
		{paginatorParams{[]int{1}, 4, 0, true}, 1, 1, []int{1}},
		{paginatorParams{[]int{1}, 4, 1, true}, 1, 1, []int{1}},
		{paginatorParams{[]int{1}, 4, 2, true}, 1, 1, []int{1}},
		// Zero items, varying orphans, no empty first page.
		{paginatorParams{[]int{}, 4, 0, false}, 0, 0, []int{}},
		{paginatorParams{[]int{}, 4, 1, false}, 0, 0, []int{}},
		{paginatorParams{[]int{}, 4, 2, false}, 0, 0, []int{}},
		// Zero items, varying orphans, allow empty first page.
		{paginatorParams{[]int{}, 4, 0, true}, 0, 1, []int{1}},
		{paginatorParams{[]int{}, 4, 1, true}, 0, 1, []int{1}},
		{paginatorParams{[]int{}, 4, 2, true}, 0, 1, []int{1}},
		// Number if items one less than per_page.
		{paginatorParams{[]int{}, 1, 0, true}, 0, 1, []int{1}},
		{paginatorParams{[]int{}, 1, 0, false}, 0, 0, []int{}},
		{paginatorParams{[]int{1}, 2, 0, true}, 1, 1, []int{1}},
		{paginatorParams{nine, 10, 0, true}, 9, 1, []int{1}},
		// Number if items equal to per_page.
		{paginatorParams{[]int{1}, 1, 0, true}, 1, 1, []int{1}},
		{paginatorParams{[]int{1, 2}, 2, 0, true}, 2, 1, []int{1}},
		{paginatorParams{ten, 10, 0, true}, 10, 1, []int{1}},
		// Number if items one more than per_page.
		{paginatorParams{[]int{1, 2}, 1, 0, true}, 2, 2, []int{1, 2}},
		{paginatorParams{[]int{1, 2, 3}, 2, 0, true}, 3, 2, []int{1, 2}},
		{paginatorParams{eleven, 10, 0, true}, 11, 2, []int{1, 2}},
		// Number if items one more than per_page with one orphan.
		{paginatorParams{[]int{1, 2, 3}, 2, 1, true}, 3, 1, []int{1}},
		{paginatorParams{eleven, 10, 1, true}, 11, 1, []int{1}},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			paginator := newIntPaginator(tc.params)
			count, err := paginator.Count(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, count, tc.count)
			numPages, err := paginator.NumPages(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, numPages, tc.numPages)
			pageRange, err := paginator.PageRange(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, pageRange, tc.pageRange)
		})
	}
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_invalid_page_number
func TestInvalidPageNumber(t *testing.T) {
	ctx := context.Background()
	paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2)
	_, err := paginator.Page(ctx, 3)
	djangotest.AssertErrorIs(t, err, views.ErrInvalidPage)
	for _, number := range []any{nil, "x", 1.2} {
		_, err = paginator.ValidateNumber(ctx, number)
		djangotest.AssertErrorIs(t, err, views.ErrPageNotAnInteger)
		djangotest.AssertErrorIs(t, err, views.ErrInvalidPage)
	}
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_orphans_value_larger_than_per_page_value
//
// Django warns that orphans >= per_page is deprecated. Go has no warning
// channel, so the paginator keeps accepting the value and the test asserts the
// resulting page count.
func TestOrphansValueLargerThanPerPageValue(t *testing.T) {
	for _, orphans := range []int{2, 3} {
		paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2, views.WithOrphans(orphans))
		numPages, err := paginator.NumPages(context.Background())
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, numPages, 1)
	}
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_error_messages
func TestErrorMessages(t *testing.T) {
	ctx := context.Background()
	assertPageError := func(p *views.Paginator[int], number any, kind error, msg string) {
		t.Helper()
		_, err := p.ValidateNumber(ctx, number)
		djangotest.AssertErrorIs(t, err, kind)
		if err != nil {
			djangotest.AssertEqual(t, err.Error(), msg)
		}
	}
	paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2, views.WithErrorMessages(map[string]string{
		"invalid_page": "Wrong page number",
		"min_page":     "Too small",
		"no_results":   "There is nothing here",
	}))
	assertPageError(paginator, 1.2, views.ErrPageNotAnInteger, "Wrong page number")
	assertPageError(paginator, -1, views.ErrEmptyPage, "Too small")
	assertPageError(paginator, 3, views.ErrEmptyPage, "There is nothing here")

	paginator = views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2,
		views.WithErrorMessages(map[string]string{"min_page": "Too small"}))
	// Custom message.
	assertPageError(paginator, -1, views.ErrEmptyPage, "Too small")
	// Default message.
	assertPageError(paginator, 3, views.ErrEmptyPage, "That page contains no results")

	// The remaining default messages, which Django's test does not spell out.
	paginator = views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2)
	assertPageError(paginator, "x", views.ErrPageNotAnInteger, "That page number is not an integer")
	assertPageError(paginator, 0, views.ErrEmptyPage, "That page number is less than 1")
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_float_integer_page
func TestFloatIntegerPage(t *testing.T) {
	paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2)
	n, err := paginator.ValidateNumber(context.Background(), 1.0)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, n, 1)
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_no_content_allow_empty_first_page
func TestNoContentAllowEmptyFirstPage(t *testing.T) {
	// With no content and allow_empty_first_page=True, 1 is a valid page.
	paginator := views.NewPaginator(views.SliceList([]int{}), 2)
	n, err := paginator.ValidateNumber(context.Background(), 1)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, n, 1)
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_paginate_misc_classes
//
// Django accepts any object with count() or __len__(). The Go counterpart is
// any implementation of views.ObjectList.
func TestPaginateMiscClasses(t *testing.T) {
	ctx := context.Background()
	paginator := views.NewPaginator[int](countContainer{n: 42}, 10)
	count, err := paginator.Count(ctx)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, count, 42)
	numPages, err := paginator.NumPages(ctx)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, numPages, 5)
	pageRange, err := paginator.PageRange(ctx)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, pageRange, []int{1, 2, 3, 4, 5})
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_count_does_not_silence_attribute_error
// Ported from Django: tests/pagination/tests.py::PaginationTests.test_count_does_not_silence_type_error
//
// Both Django tests check that an exception raised by count() propagates. In
// Go the error returned by ObjectList.Count is returned unchanged.
func TestCountDoesNotSilenceError(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("abc")
	paginator := views.NewPaginator[int](countContainer{err: boom}, 10)
	_, err := paginator.Count(ctx)
	djangotest.AssertErrorIs(t, err, boom)
	_, err = paginator.NumPages(ctx)
	djangotest.AssertErrorIs(t, err, boom)
	_, err = paginator.Page(ctx, 1)
	djangotest.AssertErrorIs(t, err, boom)
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_page_indexes
func TestPageIndexes(t *testing.T) {
	ctx := context.Background()
	ten := intsUpTo(10)
	tests := []struct {
		params paginatorParams
		first  [2]int
		last   [2]int
	}{
		// Ten items, varying per_page, no orphans.
		{paginatorParams{ten, 1, 0, true}, [2]int{1, 1}, [2]int{10, 10}},
		{paginatorParams{ten, 2, 0, true}, [2]int{1, 2}, [2]int{9, 10}},
		{paginatorParams{ten, 3, 0, true}, [2]int{1, 3}, [2]int{10, 10}},
		{paginatorParams{ten, 5, 0, true}, [2]int{1, 5}, [2]int{6, 10}},
		// Ten items, varying per_page, with orphans.
		{paginatorParams{ten, 3, 1, true}, [2]int{1, 3}, [2]int{7, 10}},
		{paginatorParams{ten, 3, 2, true}, [2]int{1, 3}, [2]int{7, 10}},
		{paginatorParams{ten, 5, 1, true}, [2]int{1, 5}, [2]int{6, 10}},
		{paginatorParams{ten, 5, 2, true}, [2]int{1, 5}, [2]int{6, 10}},
		// One item, varying orphans, no empty first page.
		{paginatorParams{[]int{1}, 4, 0, false}, [2]int{1, 1}, [2]int{1, 1}},
		{paginatorParams{[]int{1}, 4, 1, false}, [2]int{1, 1}, [2]int{1, 1}},
		{paginatorParams{[]int{1}, 4, 2, false}, [2]int{1, 1}, [2]int{1, 1}},
		// One item, varying orphans, allow empty first page.
		{paginatorParams{[]int{1}, 4, 0, true}, [2]int{1, 1}, [2]int{1, 1}},
		{paginatorParams{[]int{1}, 4, 1, true}, [2]int{1, 1}, [2]int{1, 1}},
		{paginatorParams{[]int{1}, 4, 2, true}, [2]int{1, 1}, [2]int{1, 1}},
		// Zero items, varying orphans, allow empty first page.
		{paginatorParams{[]int{}, 4, 0, true}, [2]int{0, 0}, [2]int{0, 0}},
		{paginatorParams{[]int{}, 4, 1, true}, [2]int{0, 0}, [2]int{0, 0}},
		{paginatorParams{[]int{}, 4, 2, true}, [2]int{0, 0}, [2]int{0, 0}},
	}
	checkIndexes := func(t *testing.T, params paginatorParams, last bool, want [2]int) {
		t.Helper()
		paginator := newIntPaginator(params)
		number := 1
		if last {
			var err error
			number, err = paginator.NumPages(ctx)
			djangotest.AssertEqual(t, err, nil)
		}
		page, err := paginator.Page(ctx, number)
		if err != nil {
			t.Fatalf("Page(%d): %v", number, err)
		}
		djangotest.AssertEqual(t, [2]int{page.StartIndex(), page.EndIndex()}, want)
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			checkIndexes(t, tc.params, false, tc.first)
			checkIndexes(t, tc.params, true, tc.last)
		})
	}
	// When no items and no empty first page, we should get EmptyPage error.
	for orphans := range 3 {
		_, err := newIntPaginator(paginatorParams{[]int{}, 4, orphans, false}).Page(ctx, 1)
		djangotest.AssertErrorIs(t, err, views.ErrEmptyPage)
	}
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_page_sequence
//
// A Go page is not a sequence itself: its items are the ObjectList slice.
func TestPageSequence(t *testing.T) {
	eleven := strings.Split("abcdefghijk", "")
	page2, err := views.NewPaginator(views.SliceList(eleven), 5, views.WithOrphans(1)).Page(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, page2.Len(), 6)
	djangotest.AssertEqual(t, slices.Contains(page2.ObjectList, "k"), true)
	djangotest.AssertEqual(t, slices.Contains(page2.ObjectList, "a"), false)
	djangotest.AssertEqual(t, strings.Join(page2.ObjectList, ""), "fghijk")
	reversed := slices.Clone(page2.ObjectList)
	slices.Reverse(reversed)
	djangotest.AssertEqual(t, strings.Join(reversed, ""), "kjihgf")
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_page_range_iterator
//
// Django returns a range object. PageRange returns a slice of ints.
func TestPageRangeIterator(t *testing.T) {
	pageRange, err := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2).PageRange(context.Background())
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, pageRange, []int{1, 2})
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_get_page
func TestGetPage(t *testing.T) {
	ctx := context.Background()
	paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2)
	page, err := paginator.GetPage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, page.Number, 1)
	djangotest.AssertEqual(t, page.ObjectList, []int{1, 2})
	// An empty page returns the last page.
	page, err = paginator.GetPage(ctx, 3)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, page.Number, 2)
	// Non-integer page returns the first page.
	page, err = paginator.GetPage(ctx, nil)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, page.Number, 1)
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_get_page_empty_object_list
func TestGetPageEmptyObjectList(t *testing.T) {
	ctx := context.Background()
	paginator := views.NewPaginator(views.SliceList([]int{}), 2)
	// An empty page returns the last page. Non-integer page returns the first
	// page.
	for _, number := range []any{1, 2, nil} {
		page, err := paginator.GetPage(ctx, number)
		if err != nil {
			t.Fatalf("GetPage(%v): %v", number, err)
		}
		djangotest.AssertEqual(t, page.Number, 1)
	}
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_get_page_empty_object_list_and_allow_empty_first_page_false
func TestGetPageEmptyObjectListAndAllowEmptyFirstPageFalse(t *testing.T) {
	paginator := views.NewPaginator(views.SliceList([]int{}), 2, views.WithAllowEmptyFirstPage(false))
	_, err := paginator.GetPage(context.Background(), 1)
	djangotest.AssertErrorIs(t, err, views.ErrEmptyPage)
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_paginator_iteration
func TestPaginatorIteration(t *testing.T) {
	paginator := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2)
	var lists [][]int
	var names []string
	for page, err := range paginator.Pages(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		lists = append(lists, page.ObjectList)
		names = append(names, page.String())
	}
	djangotest.AssertEqual(t, lists, [][]int{{1, 2}, {3}})
	djangotest.AssertEqual(t, names, []string{"<Page 1 of 2>", "<Page 2 of 2>"})
}

// Ported from Django: tests/pagination/tests.py::PaginationTests.test_get_elided_page_range
//
// The mock assertion that validate_number is called becomes an assertion that
// an invalid number is rejected, and the generator type checks are dropped:
// GetElidedPageRange returns a slice.
func TestGetElidedPageRange(t *testing.T) {
	ctx := context.Background()
	e := views.Ellipsis
	hundreds := func(pages, extra int) *views.Paginator[int] {
		return views.NewPaginator(views.SliceList(make([]int, pages*100+extra)), 100)
	}
	elided := func(t *testing.T, p *views.Paginator[int], number, onEachSide, onEnds int) []any {
		t.Helper()
		got, err := p.GetElidedPageRange(ctx, number, onEachSide, onEnds)
		if err != nil {
			t.Fatalf("GetElidedPageRange(%d, %d, %d): %v", number, onEachSide, onEnds, err)
		}
		return got
	}
	// Paginator.validate_number() must be called:
	_, err := views.NewPaginator(views.SliceList([]int{1, 2, 3}), 2).
		GetElidedPageRange(ctx, 3, views.DefaultOnEachSide, views.DefaultOnEnds)
	djangotest.AssertErrorIs(t, err, views.ErrEmptyPage)

	// Range is not elided if not enough pages when using default arguments:
	got := elided(t, hundreds(10, 0), 1, views.DefaultOnEachSide, views.DefaultOnEnds)
	djangotest.AssertEqual(t, slices.Contains(got, any(e)), false)
	got = elided(t, hundreds(10, 1), 1, views.DefaultOnEachSide, views.DefaultOnEnds)
	djangotest.AssertEqual(t, slices.Contains(got, any(e)), true)

	// Range should be elided if enough pages when using default arguments:
	fifty := hundreds(50, 0)
	defaults := []struct {
		number int
		want   []any
	}{
		{1, []any{1, 2, 3, 4, e, 49, 50}},
		{6, []any{1, 2, 3, 4, 5, 6, 7, 8, 9, e, 49, 50}},
		{7, []any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, e, 49, 50}},
		{8, []any{1, 2, e, 5, 6, 7, 8, 9, 10, 11, e, 49, 50}},
		{43, []any{1, 2, e, 40, 41, 42, 43, 44, 45, 46, e, 49, 50}},
		{44, []any{1, 2, e, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50}},
		{45, []any{1, 2, e, 42, 43, 44, 45, 46, 47, 48, 49, 50}},
		{50, []any{1, 2, e, 47, 48, 49, 50}},
	}
	for _, tc := range defaults {
		djangotest.AssertEqual(t, elided(t, fifty, tc.number, views.DefaultOnEachSide, views.DefaultOnEnds), tc.want)
	}

	// Range is not elided if not enough pages when using custom arguments:
	notElided := []struct{ pages, onEachSide, onEnds, elidedAfter int }{
		{6, 2, 1, 1},
		{8, 1, 3, 1},
		{8, 4, 0, 1},
		{4, 1, 1, 1},
		// When on_each_side and on_ends are both <= 1 but not both == 1 it
		// is a special case where the range is not elided until an extra
		// page is added.
		{2, 0, 1, 2},
		{2, 1, 0, 2},
		{1, 0, 0, 2},
	}
	for _, tc := range notElided {
		for offset := 0; offset <= tc.elidedAfter; offset++ {
			got := elided(t, hundreds(tc.pages+offset, 0), 1, tc.onEachSide, tc.onEnds)
			djangotest.AssertEqual(t, slices.Contains(got, any(e)), offset >= tc.elidedAfter)
		}
	}

	// Range should be elided if enough pages when using custom arguments:
	custom := []struct {
		number, onEachSide, onEnds int
		want                       []any
	}{
		// on_each_side=2, on_ends=1
		{1, 2, 1, []any{1, 2, 3, e, 50}},
		{4, 2, 1, []any{1, 2, 3, 4, 5, 6, e, 50}},
		{5, 2, 1, []any{1, 2, 3, 4, 5, 6, 7, e, 50}},
		{6, 2, 1, []any{1, e, 4, 5, 6, 7, 8, e, 50}},
		{45, 2, 1, []any{1, e, 43, 44, 45, 46, 47, e, 50}},
		{46, 2, 1, []any{1, e, 44, 45, 46, 47, 48, 49, 50}},
		{47, 2, 1, []any{1, e, 45, 46, 47, 48, 49, 50}},
		{50, 2, 1, []any{1, e, 48, 49, 50}},
		// on_each_side=1, on_ends=3
		{1, 1, 3, []any{1, 2, e, 48, 49, 50}},
		{5, 1, 3, []any{1, 2, 3, 4, 5, 6, e, 48, 49, 50}},
		{6, 1, 3, []any{1, 2, 3, 4, 5, 6, 7, e, 48, 49, 50}},
		{7, 1, 3, []any{1, 2, 3, e, 6, 7, 8, e, 48, 49, 50}},
		{44, 1, 3, []any{1, 2, 3, e, 43, 44, 45, e, 48, 49, 50}},
		{45, 1, 3, []any{1, 2, 3, e, 44, 45, 46, 47, 48, 49, 50}},
		{46, 1, 3, []any{1, 2, 3, e, 45, 46, 47, 48, 49, 50}},
		{50, 1, 3, []any{1, 2, 3, e, 49, 50}},
		// on_each_side=4, on_ends=0
		{1, 4, 0, []any{1, 2, 3, 4, 5, e}},
		{5, 4, 0, []any{1, 2, 3, 4, 5, 6, 7, 8, 9, e}},
		{6, 4, 0, []any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, e}},
		{7, 4, 0, []any{e, 3, 4, 5, 6, 7, 8, 9, 10, 11, e}},
		{44, 4, 0, []any{e, 40, 41, 42, 43, 44, 45, 46, 47, 48, e}},
		{45, 4, 0, []any{e, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50}},
		{46, 4, 0, []any{e, 42, 43, 44, 45, 46, 47, 48, 49, 50}},
		{50, 4, 0, []any{e, 46, 47, 48, 49, 50}},
		// on_each_side=0, on_ends=1
		{1, 0, 1, []any{1, e, 50}},
		{2, 0, 1, []any{1, 2, e, 50}},
		{3, 0, 1, []any{1, 2, 3, e, 50}},
		{4, 0, 1, []any{1, e, 4, e, 50}},
		{47, 0, 1, []any{1, e, 47, e, 50}},
		{48, 0, 1, []any{1, e, 48, 49, 50}},
		{49, 0, 1, []any{1, e, 49, 50}},
		{50, 0, 1, []any{1, e, 50}},
		// on_each_side=0, on_ends=0
		{1, 0, 0, []any{1, e}},
		{2, 0, 0, []any{1, 2, e}},
		{3, 0, 0, []any{e, 3, e}},
		{48, 0, 0, []any{e, 48, e}},
		{49, 0, 0, []any{e, 49, 50}},
		{50, 0, 0, []any{e, 50}},
	}
	for _, tc := range custom {
		djangotest.AssertEqual(t, elided(t, fifty, tc.number, tc.onEachSide, tc.onEnds), tc.want)
	}
}

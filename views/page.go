package views

import "fmt"

// Page is one page of a Paginator, the counterpart of
// django.core.paginator.Page. Obtain it from Paginator.Page or GetPage; the
// navigation methods of a hand-built Page report no other pages.
//
// Mapping from Django: page.object_list is ObjectList, page.number is Number,
// page.paginator is Paginator, len(page) is Len, str(page) is String, and the
// methods has_next, has_previous, has_other_pages, next_page_number,
// previous_page_number, start_index and end_index keep their names in Go
// case. Indexing and iterating a page is done on ObjectList.
type Page[T any] struct {
	// ObjectList holds the items of this page.
	ObjectList []T
	// Number is the 1-based number of this page.
	Number int
	// Paginator is the paginator the page belongs to.
	Paginator *Paginator[T]

	count    int
	numPages int
}

// String returns Django's representation "<Page 2 of 5>".
func (p *Page[T]) String() string {
	return fmt.Sprintf("<Page %d of %d>", p.Number, p.numPages)
}

// Len returns the number of items on this page.
func (p *Page[T]) Len() int { return len(p.ObjectList) }

// HasNext reports whether a page follows this one.
func (p *Page[T]) HasNext() bool { return p.Number < p.numPages }

// HasPrevious reports whether a page precedes this one.
func (p *Page[T]) HasPrevious() bool { return p.Number > 1 }

// HasOtherPages reports whether there is any page besides this one.
func (p *Page[T]) HasOtherPages() bool { return p.HasPrevious() || p.HasNext() }

// NextPageNumber returns the number of the next page, or an ErrEmptyPage when
// this is the last one.
func (p *Page[T]) NextPageNumber() (int, error) { return p.neighbor(p.Number + 1) }

// PreviousPageNumber returns the number of the previous page, or an
// ErrEmptyPage when this is the first one.
func (p *Page[T]) PreviousPageNumber() (int, error) { return p.neighbor(p.Number - 1) }

// neighbor validates the number of an adjacent page against the page count
// captured when this page was built.
func (p *Page[T]) neighbor(n int) (int, error) {
	messages := defaultPageMessages
	if p.Paginator != nil {
		messages = map[string]string{
			msgMinPage:   p.Paginator.message(msgMinPage),
			msgNoResults: p.Paginator.message(msgNoResults),
		}
	}
	switch {
	case n < 1:
		return 0, pageError(ErrEmptyPage, messages[msgMinPage])
	case n > p.numPages:
		return 0, pageError(ErrEmptyPage, messages[msgNoResults])
	default:
		return n, nil
	}
}

// perPage returns the page size of the paginator, or 0 without one.
func (p *Page[T]) perPage() int {
	if p.Paginator == nil {
		return 0
	}
	return p.Paginator.PerPage
}

// StartIndex returns the 1-based index of the first item of this page within
// the whole list, or 0 when the list is empty.
func (p *Page[T]) StartIndex() int {
	if p.count == 0 {
		return 0
	}
	return p.perPage()*(p.Number-1) + 1
}

// EndIndex returns the 1-based index of the last item of this page within the
// whole list. On the last page that is the total count, which accounts for
// orphans.
func (p *Page[T]) EndIndex() int {
	if p.Number == p.numPages {
		return p.count
	}
	return p.Number * p.perPage()
}

// TemplateValue returns the template view of the page, a map with Django's
// names: number, object_list, has_next, has_previous, has_other_pages,
// next_page_number, previous_page_number, start_index, end_index and
// paginator (see Paginator.TemplateValue). Write
//
//	{% for obj in page_obj.object_list %}
//
// where a Django template iterates page_obj itself, because the template
// engine cannot iterate a value and read attributes from it at the same time.
// next_page_number and previous_page_number are numbers when such a page
// exists; using them without one fails the render with an ErrEmptyPage, as it
// raises EmptyPage in Django.
func (p *Page[T]) TemplateValue() any {
	objects := make([]any, len(p.ObjectList))
	for i, obj := range p.ObjectList {
		if p.Paginator != nil && p.Paginator.TemplateObject != nil {
			objects[i] = p.Paginator.TemplateObject(obj)
		} else {
			objects[i] = obj
		}
	}
	out := map[string]any{
		"number":          p.Number,
		"object_list":     objects,
		"has_next":        p.HasNext(),
		"has_previous":    p.HasPrevious(),
		"has_other_pages": p.HasOtherPages(),
		"start_index":     p.StartIndex(),
		"end_index":       p.EndIndex(),
	}
	if p.HasNext() {
		out["next_page_number"] = p.Number + 1
	} else {
		out["next_page_number"] = p.NextPageNumber
	}
	if p.HasPrevious() {
		out["previous_page_number"] = p.Number - 1
	} else {
		out["previous_page_number"] = p.PreviousPageNumber
	}
	if p.Paginator != nil {
		out["paginator"] = p.Paginator.TemplateValue()
	}
	return out
}

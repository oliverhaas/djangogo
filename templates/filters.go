package templates

import (
	"fmt"

	"github.com/flosch/pongo2/v6"
)

// registerFilters installs the Django filters of this package: the ones pongo2
// lacks, and replacements for the ones it renders differently from Django.
func registerFilters() {
	for name, fn := range map[string]pongo2.FilterFunction{
		"escape":           filterEscape,
		"e":                filterEscape,
		"force_escape":     filterForceEscape,
		"safe":             filterSafe,
		"title":            filterTitle,
		"slugify":          filterSlugify,
		"truncatechars":    filterTruncatechars,
		"truncatewords":    filterTruncatewords,
		"wordwrap":         filterWordwrap,
		"stringformat":     filterStringformat,
		"pluralize":        filterPluralize,
		"urlencode":        filterURLEncode,
		"iriencode":        filterIRIEncode,
		"filesizeformat":   filterFilesizeformat,
		"intcomma":         filterIntcomma,
		"json_script":      filterJSONScript,
		"dictsort":         dictsortFilter(false),
		"dictsortreversed": dictsortFilter(true),
		"date":             dateFilter("DATE_FORMAT"),
		"time":             dateFilter("TIME_FORMAT"),
		"timesince":        filterTimesince,
		"timeuntil":        filterTimeuntil,
		"add":              filterAdd,
		"get_digit":        filterGetDigit,
		"yesno":            filterYesno,
		"floatformat":      filterFloatformat,
		"center":           filterCenter,
		"striptags":        filterStriptags,
		"safeseq":          seqFilter(false),
		"escapeseq":        seqFilter(true),

		"truncatechars_html": truncateHTMLFilter(false),
		"truncatewords_html": truncateHTMLFilter(true),

		"lower":         filterLower,
		"capfirst":      filterCapfirst,
		"addslashes":    filterAddslashes,
		"ljust":         justify("ljust", true),
		"rjust":         justify("rjust", false),
		"cut":           filterCut,
		"escapejs":      filterEscapejs,
		"phone2numeric": filterPhone2numeric,
		"slice":         filterSlice,
		"last":          filterLast,
		"random":        filterRandom,
		outFilter:       filterOut,

		safeLiteralFilter: filterSafeLiteral,

		literalPrefix + "join":            joinFilter(true, true),
		literalPrefix + "default":         filterDefaultLiteral,
		literalPrefix + "default_if_none": filterDefaultIfNoneLiteral,
		"_dict_items":                     dictViewFilter("items"),
		"_dict_keys":                      dictViewFilter("keys"),
		"_dict_values":                    dictViewFilter("values"),
	} {
		mustFilter(name, fn)
	}
	for name, fn := range autoescapeFilters {
		mustFilter(name, fn(true))
		mustFilter(noEscapePrefix+name, fn(false))
	}
}

// mustFilter registers a filter, replacing the pongo2 filter of the same name
// if there is one. pongo2 rejects neither case for a valid name, so a failure
// is a programming error.
func mustFilter(name string, fn pongo2.FilterFunction) {
	var err error
	if pongo2.FilterExists(name) {
		err = pongo2.ReplaceFilter(name, fn)
	} else {
		err = pongo2.RegisterFilter(name, fn)
	}
	if err != nil {
		panic(fmt.Sprintf("templates: register filter %q: %v", name, err))
	}
}

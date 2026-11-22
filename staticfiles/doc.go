// Package staticfiles finds, serves and collects static files, the counterpart
// of django.contrib.staticfiles. It imports the standard library only.
//
// # Mapping from Django
//
// Django finds static files with two finders: FileSystemFinder searches the
// directories of STATICFILES_DIRS and AppDirectoriesFinder searches the
// static/ directory of every installed app. Here both kinds of location are a
// Source, and one Finder searches an ordered list of them. The first source
// that holds a path wins, so a project lists its own directories before the
// apps, as Django's default STATICFILES_FINDERS does:
//
//	STATICFILES_DIRS = ["/srv/assets"]             staticfiles.DirSource("/srv/assets")
//	STATICFILES_DIRS = [("downloads", "/srv/dl")]  a DirSource with Prefix "downloads"
//	the static/ directory of an app                staticfiles.Source{Name: label, FS: embedded}
//	finders.find(path)                             finder.Find(path)
//	finders.find(path, find_all=True)              finder.FindAll(path)
//	finder.list(ignore_patterns)                   finder.List(patterns...)
//	the serve view, staticfiles_urlpatterns()      staticfiles.Handler(staticURL, finder)
//	StaticFilesHandler (runserver)                 staticfiles.Middleware(staticURL, finder)
//	manage.py collectstatic                        staticfiles.Collect(finder, options)
//	manage.py findstatic                           staticfiles.FindStaticOutput(finder, ...)
//
// A source is an fs.FS, so an app can embed its static files in the binary. A
// directory on disk is a source with a Root, which is what lets the output
// name the absolute path of a file as Django does; a file of an embedded
// source is named "<source name>:<path>".
//
// # Differences from Django
//
// Django's serve view answers 404 unless DEBUG is on. Handler has no such
// switch: the caller decides whether to mount it. It also answers 405 to
// methods other than GET and HEAD, where Django's view ignores the method.
//
// Collect copies or links into a directory. With Link, a file of an embedded
// source is copied, because it has no path to link to. Storage backends and
// post-processing (hashed file names, the manifest) are not part of this
// package, and neither are the confirmation prompt and the argument parsing
// of the collectstatic command, which belong to the caller.
//
// Django reports a misconfigured STATICFILES_DIRS through its system checks.
// Here a directory that does not exist is an empty source, a prefix may end
// with a slash, and Collect itself refuses a STATIC_ROOT that is also a source.
package staticfiles

// Package messages is the port of django.contrib.messages: one-time
// notifications for the user, such as "Your profile was saved", that a
// handler adds and the next page shows.
//
// A handler adds a message with Add or one of the shortcuts Debug, Info,
// Success, Warning and Error. The messages live in the session under the key
// "_messages", in Django's format, until a request reads them with Get or a
// template iterates over them. Reading marks them consumed: the middleware
// then keeps only the messages added after the read.
//
// Middleware must run inside the session middleware, which has to load the
// session first. ContextProcessor adds "messages" and
// "DEFAULT_MESSAGE_LEVELS" to the template context, so a base template can
// show the messages:
//
//	{% for message in messages %}
//	  <li class="{{ message.tags }}">{{ message }}</li>
//	{% endfor %}
//
// Settings come in through Configure; the zero Config is Django's default.
// Only the session storage of Django is ported: there is no cookie storage
// and no fallback storage.
package messages

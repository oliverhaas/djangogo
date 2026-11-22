// Package messages is the port of django.contrib.messages: one-time
// notifications for the user, such as "Your profile was saved", that a
// handler adds and the next page shows.
//
// A handler adds a message with Add or one of the shortcuts Debug, Info,
// Success, Warning and Error. The messages are kept until a request reads
// them with Get or a template iterates over them. Reading marks them
// consumed: the middleware then keeps only the messages added after the read.
//
// Where the messages are kept between requests depends on the storage, in
// Django's formats, so a Django process that shares the secret or the
// session reads them:
//
//   - StorageSession keeps them in the session under the key "_messages"
//     (Django's SessionStorage). Middleware uses it.
//   - StorageCookie keeps them in the signed cookie "messages" and drops the
//     oldest ones that do not fit into MaxCookieSize (Django's
//     CookieStorage).
//   - StorageFallback keeps them in the cookie and the ones that do not fit
//     there in the session (Django's FallbackStorage). An application
//     created with djangogo.New uses it unless the MESSAGE_STORAGE setting
//     selects another one.
//
// MiddlewareWithOptions selects the storage. The session and fallback
// storages need the session middleware outside the message middleware, so
// that the session is loaded first. ContextProcessor adds "messages" and
// "DEFAULT_MESSAGE_LEVELS" to the template context, so a base template can
// show the messages:
//
//	{% for message in messages %}
//	  <li class="{{ message.tags }}">{{ message }}</li>
//	{% endfor %}
//
// Settings come in through Configure; the zero Config is Django's default.
package messages

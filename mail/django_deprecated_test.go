package mail

// The whole of tests/mail/test_deprecated.py covers names and behaviors that
// Django removes in 7.0 and that the Go package never had.

// No Go counterpart: tests/mail/test_deprecated.py::DeprecationWarningTests.test_deprecated_on_import
// checks the warnings of importing BadHeaderError, SafeMIMEText and
// SafeMIMEMultipart, which have no Go counterpart.

// No Go counterpart: tests/mail/test_deprecated.py::DeprecationWarningTests.test_sanitize_address_deprecated
// checks the warning of sanitize_address(), which has no Go counterpart.

// No Go counterpart: tests/mail/test_deprecated.py::DeprecationWarningTests.test_forbid_multi_line_headers_deprecated
// checks the warning of forbid_multi_line_headers(), which has no Go
// counterpart.

// No Go counterpart: tests/mail/test_deprecated.py::UndocumentedFeatureErrorTests.test_undocumented_mixed_subtype
// sets an attribute that Message does not declare, which Go refuses at
// compile time.

// No Go counterpart: tests/mail/test_deprecated.py::UndocumentedFeatureErrorTests.test_undocumented_alternative_subtype
// sets an attribute that Message does not declare, which Go refuses at
// compile time.

// No Go counterpart: tests/mail/test_deprecated.py::DeprecatedCompatibilityTests.test_bad_header_error
// catches BadHeaderError as a ValueError; the Go errors have no such
// hierarchy.

// No Go counterpart: tests/mail/test_deprecated.py::DeprecatedCompatibilityTests.test_attachments_mimebase_in_constructor
// passes a Python MIMEText object as an attachment. Attachment.Part takes
// its place.

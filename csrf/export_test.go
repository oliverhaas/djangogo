package csrf

// Internals that the tests ported from Django's CsrfFunctionTests exercise.
var (
	UnmaskToken  = unmaskToken
	MaskSecret   = maskSecret
	CheckFormat  = checkFormat
	TokenMatches = tokenMatches
)

// Sizes of secrets and tokens for the ported tests.
const (
	SecretLength = secretLength
	TokenLength  = tokenLength
	AllowedChars = allowedChars
)

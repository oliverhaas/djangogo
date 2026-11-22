package sessions

// ErrInvalidKey is the error of a key with characters that generated keys do
// not have.
var ErrInvalidKey = errInvalidKey

// FileStorePath returns the name of the file of a key, as Django's tests call
// _key_to_file.
var FileStorePath = (*FileStore).path

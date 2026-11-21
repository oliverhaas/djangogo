package forms

import "github.com/oliverhaas/djangogo/validators"

// DomainNameValidator returns a validator for a domain name, Django's
// DomainNameValidator. With acceptIDNA false only ASCII names pass.
func DomainNameValidator(acceptIDNA bool) Validator {
	return validators.DomainNameValidator(acceptIDNA)
}

// ValidateDomainName accepts a domain name including internationalized ones,
// Django's validate_domain_name.
var ValidateDomainName = validators.ValidateDomainName

// ValidateIPv4Address accepts a dotted IPv4 address, Django's
// validate_ipv4_address.
func ValidateIPv4Address(value any) error {
	return validators.ValidateIPv4Address(value)
}

// ValidateIPv6Address accepts an IPv6 address, Django's validate_ipv6_address.
func ValidateIPv6Address(value any) error {
	return validators.ValidateIPv6Address(value)
}

// ValidateIPv46Address accepts an IPv4 or IPv6 address, Django's
// validate_ipv46_address.
func ValidateIPv46Address(value any) error {
	return validators.ValidateIPv46Address(value)
}

// EmailValidator returns a validator for an email address, Django's
// EmailValidator.
func EmailValidator(allowlist ...string) Validator {
	return validators.EmailValidator(allowlist...)
}

// ValidateEmail accepts an email address, Django's validate_email.
var ValidateEmail = validators.ValidateEmail

// URLValidator returns a validator for a URL, Django's URLValidator.
func URLValidator(schemes ...string) Validator {
	return validators.URLValidator(schemes...)
}

// ValidateURL accepts an http, https, ftp or ftps URL.
var ValidateURL = validators.ValidateURL

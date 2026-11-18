package auth

// CommonPasswords returns the embedded list of common passwords, for the
// test that checks its form.
func CommonPasswords() map[string]struct{} { return commonPasswords() }

// TemplatePermsOf exposes templatePerms.
var TemplatePermsOf = templatePerms

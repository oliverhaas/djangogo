package auth

import "github.com/oliverhaas/djangogo/urls"

// URLs returns the routes of the auth views at the paths and names of
// django.contrib.auth.urls: "login/", "logout/", "password_change/" and
// "password_change/done/". Mount them under the prefix of Config.LoginURL,
// for example with urls.Include("accounts/", "", auth.URLs()...) for the
// default "/accounts/login/". There are no password reset views.
func URLs() []urls.Route {
	return []urls.Route{
		urls.Path("login/", &LoginView{}, "login"),
		urls.Path("logout/", &LogoutView{}, "logout"),
		urls.Path("password_change/", &PasswordChangeView{}, "password_change"),
		urls.Path("password_change/done/", &PasswordChangeDoneView{}, "password_change_done"),
	}
}

package auth

import "github.com/oliverhaas/djangogo/urls"

// URLs returns the routes of the auth views at the paths and names of
// django.contrib.auth.urls: "login/", "logout/", "password_change/",
// "password_change/done/", "password_reset/", "password_reset/done/",
// "reset/<uidb64>/<token>/" and "reset/done/". Mount them under the prefix of
// Config.LoginURL, for example with urls.Include("accounts/", "",
// auth.URLs()...) for the default "/accounts/login/".
func URLs() []urls.Route {
	return []urls.Route{
		urls.Path("login/", &LoginView{}, "login"),
		urls.Path("logout/", &LogoutView{}, "logout"),
		urls.Path("password_change/", &PasswordChangeView{}, "password_change"),
		urls.Path("password_change/done/", &PasswordChangeDoneView{}, "password_change_done"),
		urls.Path("password_reset/", &PasswordResetView{}, "password_reset"),
		urls.Path("password_reset/done/", &PasswordResetDoneView{}, "password_reset_done"),
		urls.Path("reset/<uidb64>/<token>/", &PasswordResetConfirmView{}, "password_reset_confirm"),
		urls.Path("reset/done/", &PasswordResetCompleteView{}, "password_reset_complete"),
	}
}

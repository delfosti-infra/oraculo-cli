package types

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthUser struct {
	RefId    string `json:"refId"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type AuthSession struct {
	AccessToken string   `json:"accessToken"`
	User        AuthUser `json:"user"`
}

type ErrorResponse struct {
	Message    string `json:"message"`
	Error      string `json:"error"`
	StatusCode int    `json:"statusCode"`
}

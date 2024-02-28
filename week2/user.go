package web

import (
	"gitee.com/geekbang/basic-go/webook/internal/domain"
	"gitee.com/geekbang/basic-go/webook/internal/errs"
	"gitee.com/geekbang/basic-go/webook/internal/service"
	ijwt "gitee.com/geekbang/basic-go/webook/internal/web/jwt"
	regexp "github.com/dlclark/regexp2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
)

const biz = "login"

// Ensure UserHandler implements the handler interface
var _ handler = &UserHandler{}

// This is more elegant
var _ handler = (*UserHandler)(nil)

// UserHandler is used to define routes related to users
type UserHandler struct {
	svc         service.UserService
	codeSvc     service.CodeService
	emailExp    *regexp.Regexp
	passwordExp *regexp.Regexp
	ijwt.Handler
	cmd redis.Cmdable
}

func NewUserHandler(svc service.UserService,
	codeSvc service.CodeService, jwtHdl ijwt.Handler) *UserHandler {
	const (
		emailRegexPattern    = "^\\w+([-+.]\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$"
		passwordRegexPattern = `^(?=.*[A-Za-z])(?=.*\d)(?=.*[$@$!%*#?&])[A-Za-z\d$@$!%*#?&]{8,}$`
	)
	emailExp := regexp.MustCompile(emailRegexPattern, regexp.None)
	passwordExp := regexp.MustCompile(passwordRegexPattern, regexp.None)
	return &UserHandler{
		svc:         svc,
		emailExp:    emailExp,
		passwordExp: passwordExp,
		codeSvc:     codeSvc,
		Handler:     jwtHdl,
	}
}

func (u *UserHandler) RegisterRoutesV1(ug *gin.RouterGroup) {
	ug.GET("/profile", u.Profile)
	ug.POST("/signup", u.SignUp)
	ug.POST("/login", u.Login)
	ug.POST("/edit", u.Edit)
}

func (u *UserHandler) RegisterRoutes(server *gin.Engine) {
	ug := server.Group("/users")
	ug.GET("/profile", u.ProfileJWT)
	ug.POST("/signup", u.SignUp)
	//ug.POST("/login", u.Login)
	ug.POST("/login", u.LoginJWT)
	ug.POST("/logout", u.LogoutJWT)
	ug.POST("/edit", u.Edit)
	// PUT "/login/sms/code" 发验证码
	// POST "/login/sms/code" 校验验证码
	// POST /sms/login/code
	// POST /code/sms
	ug.POST("/login_sms/code/send", u.SendLoginSMSCode)
	ug.POST("/login_sms", u.LoginSMS)
	ug.POST("/refresh_token", u.RefreshToken)
}

func (u *UserHandler) LogoutJWT(ctx *gin.Context) {
	err := u.ClearToken(ctx)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "Failed to logout",
		})
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "Logout successful",
	})
}

// RefreshToken can refresh both long and short tokens, and use redis to record validity, i.e., refresh_token is disposable
// Refer to the login verification section to enhance security by comparing User-Agent
func (u *UserHandler) RefreshToken(ctx *gin.Context) {
	ctx.Request.Context()
	refreshToken := u.ExtractToken(ctx)
	var rc ijwt.RefreshClaims
	token, err := jwt.ParseWithClaims(refreshToken, &rc, func(token *jwt.Token) (interface{}, error) {
		return ijwt.RtKey, nil
	})
	if err != nil || !token.Valid {
		zap.L().Error("System error", zap.Error(err))
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	err = u.CheckSession(ctx, rc.Ssid)
	if err != nil {
		zap.L().Error("System error", zap.Error(err))
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	err = u.SetJWTToken(ctx, rc.Uid, rc.Ssid)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		zap.L().Error("System error",
			zap.Error(err),
			zap.String("method", "UserHandler:RefreshToken"))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "Refresh successful",
	})
}

func (u *UserHandler) LoginSMS(ctx *gin.Context) {
	type Req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	var req Req
	if err := ctx.Bind(&req); err != nil {
		return
	}
	ok, err := u.codeSvc.Verify(ctx, biz, req.Phone, req.Code)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "System error",
		})
		zap.L().Error("Error verifying code", zap.Error(err),
			zap.String("Phone", req.Phone))
		zap.L().Debug("", zap.String("Phone", req.Phone))
		return
	}
	if !ok {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "Incorrect code",
		})
		return
	}
	user, err := u.svc.FindOrCreate(ctx, req.Phone)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "System error",
		})
		return
	}
	if err = u.SetLoginToken(ctx, user.Id); err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "System error",
		})
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "Code verification passed",
	})
}

func (u *UserHandler) SendLoginSMSCode(ctx *gin.Context) {
	type Req struct {
		Phone string `json:"phone"`
	}
	var req Req
	if err := ctx.Bind(&req); err != nil {
		return
	}
	if req.Phone == "" {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "Input error",
		})
		return
	}
	err := u.codeSvc.Send(ctx, biz, req.Phone)
	switch err {
	case nil:
		ctx.JSON(http.StatusOK, Result{
			Msg: "Send successful",
		})
	case service.ErrCodeSendTooMany:
		zap.L().Warn("SMS sending too frequently",
			zap.Error(err))
		ctx.JSON(http.StatusOK, Result{
			Msg: "Sending too frequently, please try again later",
		})
	default:
		zap.L().Error("SMS sending failed",
			zap.Error(err))
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "System error",
		})
	}
}

func (u *UserHandler) SignUp(ctx *gin.Context) {
	type SignUpReq struct {
		Email           string `json:"email"`
		ConfirmPassword string `json:"confirmPassword"`
		Password        string `json:"password"`
	}
	var req SignUpReq
	if err := ctx.Bind(&req); err != nil {
		return
	}
	ok, err := u.emailExp.MatchString(req.Email)
	if err != nil {
		ctx.String(http.StatusOK, "System error")
		return
	}
	if !ok {
		ctx.String(http.StatusOK, "Invalid email format")
		return
	}
	if req.ConfirmPassword != req.Password {
		ctx.String(http.StatusOK, "Passwords do not match")
		return
	}
	ok, err = u.passwordExp.MatchString(req.Password)
	if err != nil {
		ctx.String(http.StatusOK, "System error")
		return
	}
	if !ok {
		ctx.String(http.StatusOK, "Password must be at least 8 characters long and contain a number and a special character")
		return
	}
	err = u.svc.SignUp(ctx.Request.Context(), domain.User{
		Email:    req.Email,
		Password: req.Password,
	})
	if err == service.ErrUserDuplicateEmail {
		span := trace.SpanFromContext(ctx.Request.Context())
		span.AddEvent("Email conflict")
		ctx.String(http.StatusOK, "Email conflict")
		return
	}
	if err != nil {
		ctx.String(http.StatusOK, "System exception")
		return
	}
	ctx.String(http.StatusOK, "Registration successful")
}

func (u *UserHandler) LoginJWT(ctx *gin.Context) {
	type LoginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req LoginReq
	if err := ctx.Bind(&req); err != nil {
		return
	}
	user, err := u.svc.Login(ctx, req.Email, req.Password)
	if err == service.ErrInvalidUserOrPassword {
		ctx.String(http.StatusOK, "Invalid username or password")
		return
	}
	if err != nil {
		ctx.String(http.StatusOK, "System error")
		return
	}
	if err = u.SetLoginToken(ctx, user.Id); err != nil {
		ctx.String(http.StatusOK, "System error")
		return
	}
	ctx.String(http.StatusOK, "Login successful")
	return
}

func (u *UserHandler) Login(ctx *gin.Context) {
	type LoginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req LoginReq
	if err := ctx.Bind(&req); err != nil {
		return
	}
	user, err := u.svc.Login(ctx, req.Email, req.Password)
	if err == service.ErrInvalidUserOrPassword {
		ctx.JSON(http.StatusOK, Result{
			Code: errs.UserInvalidOrPassword,
			Msg:  "User does not exist or incorrect password",
		})
		return
	}
	if err != nil {
		ctx.String(http.StatusOK, "System error")
		return
	}
	sess := sessions.Default(ctx)
	sess.Set("userId", user.Id)
	sess.Options(sessions.Options{
		Secure:   true,
		HttpOnly: true,
		MaxAge:   60,
	})
	sess.Save()
	ctx.String(http.StatusOK, "Login successful")
	return
}

func (u *UserHandler) Logout(ctx *gin.Context) {
	sess := sessions.Default(ctx)
	sess.Options(sessions.Options{
		MaxAge: -1,
	})
	sess.Save()
	ctx.String(http.StatusOK, "Logout successful")
}

func (u *UserHandler) Edit(ctx *gin.Context) {

}

func (u *UserHandler) ProfileJWT(ctx *gin.Context) {
	c, _ := ctx.Get("users")
	claims, ok := c.(ijwt.UserClaims)
	if !ok {
		ctx.String(http.StatusOK, "System error")
		return
	}
	println(claims.Id)
	ctx.String(http.StatusOK, "Your profile")
}

func (u *UserHandler) Profile(ctx *gin.Context) {
	ctx.String(http.StatusOK, "This is your profile")
}

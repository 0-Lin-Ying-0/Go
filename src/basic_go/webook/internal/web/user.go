package web

import (
	"basic_go/webook/internal/domain"
	"basic_go/webook/internal/service"
	"net/http"
	"time"

	regexp "github.com/dlclark/regexp2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v5"
)

const (
	emailRegexPattern = "^\\w+([-+.]\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$"
	// 和上面比起来，用 ` 看起来就比较清爽
	passwordRegexPattern = `^(?=.*[A-Za-z])(?=.*\d)(?=.*[$@$!%*#?&])[A-Za-z\d$@$!%*#?&]{8,}$`
)

type UserHandler struct {
	emailRegex *regexp.Regexp
	password   *regexp.Regexp
	svc        *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{
		emailRegex: regexp.MustCompile(emailRegexPattern, regexp.None),
		password:   regexp.MustCompile(passwordRegexPattern, regexp.None),
		svc:        svc,
	}
}
func (h *UserHandler) RegisterRoutes(server *gin.Engine) {
	//REST风格
	//sever.POST("/user", h.SingUp)
	//sever.PUT("/user", h.SingUp)
	//sever.GET("/users/:username", h.Profile)

	//分组注册
	ug := server.Group("/users")
	//POST /users/signup
	ug.POST("/signup", h.SignUp)
	//POST /users/login
	//ug.POST("/login", h.Login)
	ug.POST("/login", h.LoginJWT)

	//POST /users/edit
	ug.POST("/edit", h.Edit)
	//GET /users/profile
	ug.GET("/profile", h.Profile)
}

func (h *UserHandler) SignUp(ctx *gin.Context) {
	type SignUpReq struct {
		Email           string `json:"email" `
		Password        string `json:"password" `
		ConfirmPassword string `json:"confirmPassword" `
	}

	var req SignUpReq
	if err := ctx.Bind(&req); err != nil {
		return
	}

	isEmail, err := h.emailRegex.MatchString(req.Email)

	if err != nil {
		ctx.String(http.StatusOK, "系统错误")
		return
	}
	if !isEmail {
		ctx.String(http.StatusOK, "非法邮箱格式")
		return
	}

	if req.Password != req.ConfirmPassword {
		ctx.String(http.StatusOK, "密码两次输入错误")
		return
	}

	isPassword, err := h.password.MatchString(req.Password)
	if err != nil {
		ctx.String(http.StatusOK, "系统错误")
		return
	}
	if !isPassword {
		ctx.String(http.StatusOK, "密码格式错误")
		return
	}

	err = h.svc.Signup(ctx, domain.User{
		Email:    req.Email,
		Password: req.Password,
	})

	switch err {
	case nil:
		ctx.String(http.StatusOK, "hello 欢迎注册")
	case service.ErrDuplicateEmail:
		ctx.String(http.StatusOK, "邮箱冲突，请换一个")
	default:
		ctx.String(http.StatusOK, "系统错误")

	}

	// 要判定邮箱冲突

}

func (h *UserHandler) LoginJWT(ctx *gin.Context) {
	type LoginReq struct {
		Email    string `json:"email" `
		Password string `json:"password" `
	}
	var req LoginReq
	if err := ctx.Bind(&req); err != nil {
		return
	}

	u, err := h.svc.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		uc := UserClaims{
			Uid: u.Id,
			RegisteredClaims: jwt.RegisteredClaims{
				// 1分钟到期
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS512, uc)
		tokenStr, err := token.SignedString(JWTKey)
		if err != nil {
			ctx.String(http.StatusOK, "系统错误")
		}
		ctx.Header("x-jwt-token", tokenStr)

		ctx.String(http.StatusOK, "登录成功")
	case service.ErrInvalidUserOrPassword:
		ctx.String(http.StatusOK, "用户名或者密码错误")
	default:
		ctx.String(http.StatusOK, "系统错误")

	}
}

func (h *UserHandler) Login(ctx *gin.Context) {
	type LoginReq struct {
		Email    string `json:"email" `
		Password string `json:"password" `
	}
	var req LoginReq
	if err := ctx.Bind(&req); err != nil {
		return
	}

	u, err := h.svc.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:

		sess := sessions.Default(ctx)
		sess.Set("userId", u.Id)
		sess.Options(sessions.Options{
			// 设置15分钟
			MaxAge: 30,
		})
		err = sess.Save()
		if err != nil {
			ctx.String(http.StatusOK, "系统错误")
			return
		}

		ctx.String(http.StatusOK, "登录成功")
	case service.ErrInvalidUserOrPassword:
		ctx.String(http.StatusOK, "用户名或者密码错误")
	default:
		ctx.String(http.StatusOK, "系统错误")

	}

}

func (h *UserHandler) Edit(ctx *gin.Context) {

}

func (h *UserHandler) Profile(ctx *gin.Context) {
	//us := ctx.MustGet("user").(UserClaims)
	ctx.String(http.StatusOK, "这是 profile")
}

var JWTKey = []byte("aNaL?A*dqgo#oE3aPjmU,AE:D2bxNtPtK4P%,kXp.*Auqpd>}c!>iun=M?AhA5XW")

type UserClaims struct {
	jwt.RegisteredClaims
	Uid int64
}

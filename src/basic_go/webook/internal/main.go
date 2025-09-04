package main

import (
	"basic_go/webook/internal/repository"
	"basic_go/webook/internal/repository/dao"
	"basic_go/webook/internal/service"
	"basic_go/webook/internal/web"
	"basic_go/webook/internal/web/middleware"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

func main() {

	db := initDB()

	server := initWebServer()
	initUserHdl(db, server)
	server.Run(":8080")
}

func initUserHdl(db *gorm.DB, server *gin.Engine) {
	ud := dao.NewUserDAO(db)
	ur := repository.NewUserRepository(ud)
	us := service.NewUserService(ur)

	hdl := web.NewUserHandler(us)
	hdl.RegisterRoutes(server)

	//server.POST("/users/signup", hdl.SignUp)
	//server.POST("/users/login", hdl.Login)
	//server.POST("/users/edit", hdl.Edit)
	//server.GET("/users/profile", hdl.Profile)
}

func initDB() *gorm.DB {
	db, err := gorm.Open(mysql.Open("root:root@tcp(localhost:13316)/webook"))
	if err != nil {
		panic(err)
	}

	err = dao.InitTables(db)
	if err != nil {
		panic(err)
	}
	return db
}

func initWebServer() *gin.Engine {
	server := gin.Default()

	server.Use(cors.New(cors.Config{
		//AllowAllOrigins:  true,
		//AllowOrigins:     []string{"https://localhost:3000"},
		//AllowMethods:     []string{"PUT", "PATCH"}, //不用配，允许所有方法就可以
		AllowHeaders: []string{"Content-Type"},
		//AllowHeaders:     []string{"content-type"},

		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			//if strings.Contains(origin, "localhost") {
			if strings.HasPrefix(origin, "http://localhost") {
				return true
			}
			return strings.Contains(origin, "you_company.com")
		},
		MaxAge: 12 * time.Hour,
	}), func(ctx *gin.Context) {
		println("这是我的middleware")
	})

	login := &middleware.LoginMiddlewareBuilder{}
	// 存储数据的，也就是你的 userId 存哪里
	// 刚开始先直接存 cookie
	//store := cookie.NewStore([]byte("secret"))

	// 基于内存的实现，第一个参数是 authentication key ,用于身份认证,最好是32 或者 64 位
	// 第二个参数是 encryption key，加密数据用的
	//store := memstore.NewStore([]byte("qfqwbxb9i5C9G_fXL:UNfU>Pm0MVyh7?*):E}WUNX2v4ww=^!k9K~j:1fXc!1VrF"),
	//	[]byte("aNaL?A*dqgo#oE3aPjmU,AE:D1bxNtPtK4P%,kXp.*Auqpd>}c!>iun=M?AhA5XW"))

	store, err := redis.NewStore(16, "tcp", "localhost:6379", "", "",
		[]byte("qfqwbxb9i5C9G_fXL:UNfU>Pm0MVyh7?*):E}WUNX2v4ww=^!k9K~j:1fXc!1VrF"),
		[]byte("qfqwbxb9i5C9G_fXL:UNfU>Pm0MVyh7?*):E}WUNX2v4ww=^!k9K~j:1fXc!1VrA"))
	if err != nil {
		panic(err)
	}
	server.Use(sessions.Sessions("ssid", store), login.CheckLogin())
	// sessions.Sessions("ssid", store) 是初始化
	return server
}

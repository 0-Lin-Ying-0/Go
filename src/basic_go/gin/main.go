package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	server := gin.Default()

	server.Use(func(context *gin.Context) {
		println("这是第一个Middleware")
	}, func(context *gin.Context) {
		println("这是第二个Middleware")
	})

	//静态路由
	server.GET("/hello", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello world")
	})

	//参数路由，路径参数
	server.GET("/users/:name", func(ctx *gin.Context) {
		name := ctx.Param("name")
		ctx.String(http.StatusOK, "hello"+name)
	})

	// 查询参数
	// GET/path?id=1234
	server.GET("/path", func(ctx *gin.Context) {
		id := ctx.Query("id")
		ctx.String(http.StatusOK, "订单 ID 是"+id)
	})

	// 通配符匹配
	server.GET("/views/*.html", func(ctx *gin.Context) {
		view := ctx.Param(".html")
		ctx.String(http.StatusOK, "viwe 是"+view)
	})

	server.POST("/login", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello login")
	})

	//go func() {
	//	server1 := gin.Default()
	//	server1.Run(":8081")
	//}
	//如果不传参数，那么实际上监听的是8080端口
	server.Run(":8080")
}

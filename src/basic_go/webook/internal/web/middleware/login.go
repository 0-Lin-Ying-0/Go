package middleware

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type LoginMiddlewareBuilder struct {
}

func (m *LoginMiddlewareBuilder) CheckLogin() gin.HandlerFunc {
	// 注册一下这个类型
	gob.Register(time.Now())
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		if path == "/users/signup" || path == "/users/login" {
			// 不需要登录校验
			return
		}
		sess := sessions.Default(ctx)
		userId := sess.Get("userId")
		if userId == nil {
			// 中断，不要往后执行，也就是不要执行后面的业务逻辑
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		now := time.Now()

		// 怎么知道要刷新了呢
		// 假如说我们的策略是每分钟刷一次，怎么知道已经过了一分钟？ 就是记录上一次刷新的时间
		const updateTimeKey = "update_time"
		// 试着拿出上一次刷新时间
		val := sess.Get(updateTimeKey)
		lastUpdateTime, ok := val.(time.Time)
		if val == nil || !ok || now.Sub(lastUpdateTime) > time.Second*10 {
			// 代表第一次进来
			sess.Set(updateTimeKey, now)
			sess.Set("userId", userId)
			err := sess.Save()
			if err != nil {
				// 打日志
				fmt.Println(err)
			}
		}
		//lastUpdateTime, ok := val.(time.Time)
		//if !ok {
		//	// 代表第一次进来
		//	sess.Set(updateTimeKey, now)
		//	sess.Set("userId", userId)
		//	err := sess.Save()
		//	if err != nil {
		//		// 打日志
		//		fmt.Println(err)
		//	}
		//}
		//
		//// 过了一分钟
		//if now.Sub(lastUpdateTime) > time.Minute {
		//	sess.Set(updateTimeKey, now)
		//	sess.Set("userId", userId)
		//	err := sess.Save()
		//	if err != nil {
		//		// 打日志
		//		fmt.Println(err)
		//	}
		//}
	}
}

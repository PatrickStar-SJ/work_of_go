//可以使用一个统一的错误处理函数来处理错误并打印日志。以下是重写后的代码示例：

func handleErr(ctx *gin.Context, err error, code int, msg string) {
	ctx.JSON(http.StatusOK, Result{
		Code: code,
		Msg:  msg,
	})
	zap.L().Error(msg, zap.Error(err))
}

func (c *Controller) Login(ctx *gin.Context) {
	type Req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	var req Req
	if err := ctx.Bind(&req); err != nil {
		handleErr(ctx, err, 5, "系统异常")
		return
	}

	ok, err := c.svc.VerifyCode(req.Phone, req.Code)
	if err != nil {
		handleErr(ctx, err, 5, "系统异常")
		return
	}
	if !ok {
		handleErr(ctx, nil, 4, "验证码错误")
		return
	}

	u, err := c.svc.FindOrCreate(ctx, req.Phone)
	if err != nil {
		handleErr(ctx, err, 4, "系统错误")
		return
	}

	// Continue with login or registration logic
	// ...
}

/*
在上面的代码中，我们定义了一个名为`handleErr`的函数，它接收错误、状态码和消息作为参数，并在处理错误时打印日志。然后，在每个可能发生错误的地方，我们都调用这个函数来处理错误并返回相应的响应。这样就避免了在每个 if-else 分支中手动打印日志的重复代码。
*/

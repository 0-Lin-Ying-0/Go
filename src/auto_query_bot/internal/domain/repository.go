package domain

// CaptchaRepository 验证码接口标准
type CaptchaRepository interface {
	Fetch() (*Captcha, error)
	Solve(c *Captcha) (string, error)
}

// DeviceRepository 设备查询接口标准
type DeviceRepository interface {
	Find(sn string, captchaCode string) (*DeviceInfo, error)
}

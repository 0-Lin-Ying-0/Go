package domain

// Captcha 验证码实体
type Captcha struct {
	ImageBytes []byte
	Value      string
}

// DeviceInfo 设备信息实体
type DeviceInfo struct {
	SN      string
	RawHTML string // 原始返回结果，供人工复核
	Status  string // 成功或失败状态
}

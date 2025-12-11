package domain

// Captcha 验证码实体
type Captcha struct {
	ImageBytes []byte
	Value      string
}

// DeviceInfo 设备信息实体
type DeviceInfo struct {
	SN           string
	HardwareHTML string // 硬件查询结果
	SoftwareHTML string // 软件查询结果
	Status       string // 成功或失败状态
}

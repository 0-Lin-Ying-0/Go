package targets

import "context"

// “把规则变成可扫描目标列表”的抽象入口

type Provider interface {
	TargetsForRule(ctx context.Context, ruleID int64, incremental bool) ([]string, error)
}

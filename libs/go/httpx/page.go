package httpx

// Page 是列表端点的强类型分页响应体。成功响应直接返回本结构，不裹任何信封。
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// NewPage 构造分页响应，并把 nil 切片归一为空切片，保证序列化为 [] 而非 null。
func NewPage[T any](items []T, total int64, page, size int) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Total: total, Page: page, Size: size}
}

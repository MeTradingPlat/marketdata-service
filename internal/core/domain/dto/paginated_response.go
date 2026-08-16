package dto

// PaginatedResponse envuelve cualquier listado paginado -- forma de
// respuesta pura, sin logica propia, mismo campo por campo que
// PaginatedResponse<T> del frontend.
type PaginatedResponse[T any] struct {
	Data          []T   `json:"data"`
	Page          int   `json:"page"`
	PageSize      int   `json:"pageSize"`
	TotalPages    int   `json:"totalPages"`
	TotalElements int64 `json:"totalElements"`
}

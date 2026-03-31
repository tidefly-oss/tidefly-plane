package shared

import "github.com/danielgtaylor/huma/v2"

func Op(id, method, path, summary, tag string, mws ...func(huma.Context, func(huma.Context))) huma.Operation {
	return huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Tags:        []string{tag},
		Middlewares: mws,
	}
}

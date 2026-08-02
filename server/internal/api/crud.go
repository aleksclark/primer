// Package api wires the LMS resources into a Huma REST API. Huma generates
// the OpenAPI 3.1 specification directly from the handler type signatures.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aleksclark/primer/server/internal/repo"
)

// ListInput is the standard query parameter set accepted by every list
// endpoint: pagination, free-text search, sorting, and exact-match filters.
type ListInput struct {
	Limit  int      `query:"limit" minimum:"1" maximum:"200" default:"25" doc:"Page size."`
	Offset int      `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
	Q      string   `query:"q" doc:"Free-text search across the resource's searchable columns."`
	Sort   string   `query:"sort" doc:"Column to sort by (whitelisted per resource)."`
	Dir    string   `query:"dir" enum:"asc,desc" default:"asc" doc:"Sort direction."`
	Filter []string `query:"filter,explode" doc:"Exact-match filters as column:value pairs, e.g. filter=status:active. Repeatable."`
}

// PageBody is the standard paginated response envelope.
type PageBody[T any] struct {
	Items      []T `json:"items"`
	TotalCount int `json:"totalCount" doc:"Total rows matching the query, ignoring pagination."`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
}

// listOutput wraps a page for Huma.
type listOutput[T any] struct {
	Body PageBody[T]
}

// itemOutput wraps a single entity for Huma.
type itemOutput[T any] struct {
	Body T
}

// getInput is the input for GET-by-id endpoints.
type getInput struct {
	ID string `path:"id" format:"uuid" doc:"Entity ID."`
}

// deleteInput is the input for DELETE endpoints.
type deleteInput struct {
	ID string `path:"id" format:"uuid" doc:"Entity ID."`
}

// createInput wraps a create request body.
type createInput[C any] struct {
	Body C
}

// updateInput wraps a partial update request body.
type updateInput[U any] struct {
	ID   string `path:"id" format:"uuid" doc:"Entity ID."`
	Body U
}

// deleteOutput is an empty 204 response.
type deleteOutput struct{}

// crudConfig records which of the standard operations to register and how
// they are guarded.
type crudConfig struct {
	skipCreate bool
	skipUpdate bool
	skipDelete bool
	middleware huma.Middlewares
	security   []map[string][]string
	errors     []int
}

// CRUDOption opts a resource out of part of the standard operation set, or
// wraps its operations in authentication.
type CRUDOption func(*crudConfig)

// Guard applies middleware to every generated operation and documents the
// security scheme it enforces, so an authenticated resource does not have to
// hand-roll its endpoints.
func Guard(middleware func(huma.Context, func(huma.Context)), scheme string) CRUDOption {
	return func(c *crudConfig) {
		c.middleware = append(c.middleware, middleware)
		if scheme != "" {
			c.security = append(c.security, map[string][]string{scheme: {}})
		}
		c.errors = append(c.errors, http.StatusUnauthorized)
	}
}

// apply stamps the configured guard onto an operation.
func (c crudConfig) apply(op huma.Operation) huma.Operation {
	op.Middlewares = append(op.Middlewares, c.middleware...)
	op.Security = append(op.Security, c.security...)
	op.Errors = append(op.Errors, c.errors...)
	return op
}

// SkipCreate omits the create endpoint, for resources whose creation needs a
// bespoke handler or that are read-only.
func SkipCreate() CRUDOption { return func(c *crudConfig) { c.skipCreate = true } }

// SkipUpdate omits the update endpoint.
func SkipUpdate() CRUDOption { return func(c *crudConfig) { c.skipUpdate = true } }

// SkipDelete omits the delete endpoint.
func SkipDelete() CRUDOption { return func(c *crudConfig) { c.skipDelete = true } }

// RegisterCRUD registers the standard list/get/create/update/delete
// operations for a resource. T is the entity, C the create body, and U the
// partial-update body (all fields must be pointers). Options omit individual
// operations for resources that are read-only or need a bespoke handler.
func RegisterCRUD[T, C, U any](api huma.API, q repo.Querier, res *repo.Resource[T], singular, plural, path string, opts ...CRUDOption) {
	tag := titleCase(plural)
	var cfg crudConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	huma.Register(api, cfg.apply(huma.Operation{
		OperationID: "list-" + plural,
		Method:      http.MethodGet,
		Path:        path,
		Summary:     "List " + plural,
		Description: fmt.Sprintf("List %s with pagination, search (%s), sorting (%s), and filters (%s).",
			plural,
			strings.Join(res.Config().SearchColumns, ", "),
			strings.Join(res.Config().SortableColumns, ", "),
			strings.Join(res.Config().FilterableColumns, ", ")),
		Tags: []string{tag},
	}), func(ctx context.Context, in *ListInput) (*listOutput[T], error) {
		filters, err := ParseFilters(in.Filter)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		page, err := res.List(ctx, q, repo.ListParams{
			Limit:   in.Limit,
			Offset:  in.Offset,
			Search:  in.Q,
			Sort:    in.Sort,
			Dir:     repo.SortDir(in.Dir),
			Filters: filters,
		})
		if err != nil {
			return nil, MapError(err)
		}
		if page.Items == nil {
			page.Items = []T{}
		}
		return &listOutput[T]{Body: PageBody[T]{
			Items:      page.Items,
			TotalCount: page.TotalCount,
			Limit:      page.Limit,
			Offset:     page.Offset,
		}}, nil
	})

	huma.Register(api, cfg.apply(huma.Operation{
		OperationID: "get-" + singular,
		Method:      http.MethodGet,
		Path:        path + "/{id}",
		Summary:     "Get a " + singular,
		Tags:        []string{tag},
	}), func(ctx context.Context, in *getInput) (*itemOutput[T], error) {
		item, err := res.Get(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[T]{Body: *item}, nil
	})

	if !cfg.skipCreate {
		huma.Register(api, cfg.apply(huma.Operation{
			OperationID:   "create-" + singular,
			Method:        http.MethodPost,
			Path:          path,
			Summary:       "Create a " + singular,
			Tags:          []string{tag},
			DefaultStatus: http.StatusCreated,
		}), func(ctx context.Context, in *createInput[C]) (*itemOutput[T], error) {
			item, err := res.Create(ctx, q, structToValues(in.Body))
			if err != nil {
				return nil, MapError(err)
			}
			return &itemOutput[T]{Body: *item}, nil
		})
	}

	if !cfg.skipUpdate {
		huma.Register(api, cfg.apply(huma.Operation{
			OperationID: "update-" + singular,
			Method:      http.MethodPatch,
			Path:        path + "/{id}",
			Summary:     "Update a " + singular,
			Description: "Partial update: only provided fields are changed.",
			Tags:        []string{tag},
		}), func(ctx context.Context, in *updateInput[U]) (*itemOutput[T], error) {
			item, err := res.Update(ctx, q, in.ID, structToValues(in.Body))
			if err != nil {
				return nil, MapError(err)
			}
			return &itemOutput[T]{Body: *item}, nil
		})
	}

	if !cfg.skipDelete {
		huma.Register(api, cfg.apply(huma.Operation{
			OperationID:   "delete-" + singular,
			Method:        http.MethodDelete,
			Path:          path + "/{id}",
			Summary:       "Delete a " + singular,
			Tags:          []string{tag},
			DefaultStatus: http.StatusNoContent,
		}), func(ctx context.Context, in *deleteInput) (*deleteOutput, error) {
			if err := res.Delete(ctx, q, in.ID); err != nil {
				return nil, MapError(err)
			}
			return &deleteOutput{}, nil
		})
	}
}

// titleCase converts "assessment-items" to "Assessment Items" for tags.
func titleCase(s string) string {
	words := strings.Split(s, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// ParseFilters converts ["col:value", ...] into a filter map.
func ParseFilters(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, f := range raw {
		col, val, ok := strings.Cut(f, ":")
		if !ok || col == "" {
			return nil, fmt.Errorf("invalid filter %q: expected column:value", f)
		}
		out[col] = val
	}
	return out, nil
}

// structToValues converts a request body struct into a column->value map
// using db tags. Nil pointer fields are omitted (unset), non-nil pointers
// are dereferenced.
func structToValues(v any) map[string]any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	t := rv.Type()
	out := make(map[string]any, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		col := t.Field(i).Tag.Get("db")
		if col == "" || col == "-" {
			continue
		}
		fv := rv.Field(i)
		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		out[col] = fv.Interface()
	}
	return out
}

// MapError translates repository and database errors into HTTP errors.
func MapError(err error) error {
	if errors.Is(err, repo.ErrNotFound) {
		return huma.Error404NotFound("resource not found")
	}
	var badReq repo.ErrBadRequest
	if errors.As(err, &badReq) {
		return huma.Error400BadRequest(badReq.Msg)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return huma.Error409Conflict("duplicate: " + pgErr.Detail)
		case "23503":
			return huma.Error422UnprocessableEntity("invalid reference: " + pgErr.Detail)
		case "23514":
			return huma.Error422UnprocessableEntity("constraint violation: " + pgErr.ConstraintName)
		case "22P02":
			return huma.Error400BadRequest("invalid input syntax: " + pgErr.Message)
		}
	}
	return err
}

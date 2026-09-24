package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是全站统一响应结构：{code, message, data}，code=0 表示成功。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Page 分页响应载荷。
type Page struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data})
}

func OKPage(c *gin.Context, list any, total int64, page, pageSize int) {
	OK(c, Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

func Fail(c *gin.Context, status, code int, msg string) {
	c.AbortWithStatusJSON(status, Body{Code: code, Message: msg})
}

func BadRequest(c *gin.Context, msg string)   { Fail(c, http.StatusBadRequest, 40000, msg) }
func Unauthorized(c *gin.Context, msg string) { Fail(c, http.StatusUnauthorized, 40100, msg) }
func Forbidden(c *gin.Context, msg string)    { Fail(c, http.StatusForbidden, 40300, msg) }
func NotFound(c *gin.Context, msg string)     { Fail(c, http.StatusNotFound, 40400, msg) }
func Error(c *gin.Context, msg string)        { Fail(c, http.StatusInternalServerError, 50000, msg) }

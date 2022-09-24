package errors

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net/http"
	"strings"
	"sync"
)

func InitErr(dsn string) {
	initDB(dsn)
	register()
	go watch(dsn)
}

type dict struct {
	mux          *sync.RWMutex
	errorCodeMap map[int]*custom
	errorNameMap map[string]*custom
}

var errorDict = dict{
	mux:          new(sync.RWMutex),
	errorCodeMap: make(map[int]*custom, 0),
	errorNameMap: make(map[string]*custom, 0),
}

func register() {
	list := getAllErrorCode()
	errorCodeMap := make(map[int]*custom, 0)
	errorNameMap := make(map[string]*custom, 0)
	for i := range list {
		customError := &custom{
			name:       list[i].Name,
			errorCode:  list[i].ErrorCode,
			grpcStatus: list[i].GrpcStatus,
			httpStatus: HttpStatus(list[i].GrpcStatus),
			message:    list[i].Message,
		}
		errorCodeMap[list[i].ErrorCode] = customError
		errorNameMap[list[i].Name] = customError
	}

	if _, ok := errorNameMap["Err_GRPC_Connection"]; !ok {
		errorNameMap["Err_GRPC_Connection"] = &custom{
			name:       "Err_GRPC_Connection",
			errorCode:  50001,
			grpcStatus: codes.Unavailable,
			httpStatus: HttpStatus(codes.Unavailable),
			message:    "无法连接至Grpc服务",
		}
	}
	if _, ok := errorNameMap["Err_Unknown"]; !ok {
		errorNameMap["Err_Unknown"] = &custom{
			name:       "Err_Unknown",
			errorCode:  50000,
			grpcStatus: codes.Unknown,
			httpStatus: HttpStatus(codes.Unknown),
			message:    "未知错误",
		}
	}
	errorDict.mux.Lock()
	defer errorDict.mux.Unlock()

	errorDict.errorCodeMap = errorCodeMap
	errorDict.errorNameMap = errorNameMap
}

func HttpStatus(c codes.Code) int {
	switch c {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.Aborted, codes.AlreadyExists:
		return http.StatusConflict
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Canceled:
		return 499
	default:
		return http.StatusInternalServerError
	}
}

type Custom interface {
	GetErrorCode() int
	GetGrpcStatus() codes.Code
	GetHttpStatus() int
	GetMessage() string
}

type custom struct {
	errorCode  int
	grpcStatus codes.Code
	httpStatus int
	name       string
	message    string
	cause      error
	*stack
}

func (w *custom) Error() string { return w.cause.Error() }
func (w *custom) Cause() error  { return w.cause }
func (w *custom) Unwrap() error { return w.cause }
func (w *custom) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v", w.Cause())
			w.stack.Format(s, verb)
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, w.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", w.Error())
	}
}

func (w *custom) GetErrorCode() int {
	return w.errorCode
}

func (w *custom) GetGrpcStatus() codes.Code {
	return w.grpcStatus
}

func (w *custom) GetHttpStatus() int {
	return w.httpStatus
}

func (w *custom) GetMessage() string {
	return w.message
}

func NewGrpcError(errName string) error {
	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()
	var customError = errorDict.errorNameMap["Err_Unknown"]
	if _, ok := errorDict.errorNameMap[errName]; ok {
		customError = errorDict.errorNameMap[errName]
	}
	return status.New(customError.GetGrpcStatus(), errName).Err()
}

func NewGrpcErrorWithCode(errCode int) error {
	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()
	var customError = errorDict.errorNameMap["Err_Unknown"]
	if _, ok := errorDict.errorCodeMap[errCode]; ok {
		customError = errorDict.errorCodeMap[errCode]
	}
	return status.New(customError.GetGrpcStatus(), customError.name).Err()
}

func ParseGrpcError(err error) error {
	grpcErr := status.Convert(err)

	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()

	// 处理尚未打到后端而发生的异常
	if grpcErr.Code() == codes.Unavailable && strings.HasPrefix(grpcErr.Message(), "connection error") {
		customErr := errorDict.errorNameMap["Err_GRPC_Connection"]
		customErr.stack = callers()
		customErr.cause = err
		return customErr
	}

	if customErr, ok := errorDict.errorNameMap[grpcErr.Message()]; !ok {
		newCustomErr := errorDict.errorNameMap["Err_Unknown"]
		newCustomErr.stack = callers()
		newCustomErr.cause = err
		return newCustomErr
	} else {
		customErr.stack = callers()
		customErr.cause = err
		return customErr
	}
}

func NewCustom(name string, errInfo any) error {
	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()

	if _, ok := errorDict.errorNameMap[name]; !ok {
		name = "Err_Unknown"
	}
	switch err := errInfo.(type) {
	case nil:
		return nil
	case *custom:
		return &custom{
			name:       name,
			errorCode:  errorDict.errorNameMap[name].errorCode,
			grpcStatus: errorDict.errorNameMap[name].grpcStatus,
			httpStatus: errorDict.errorNameMap[name].httpStatus,
			message:    errorDict.errorNameMap[name].message,
			cause:      err.cause,
			stack:      err.stack,
		}
	case *withStack:
		return &custom{
			name:       name,
			errorCode:  errorDict.errorNameMap[name].errorCode,
			grpcStatus: errorDict.errorNameMap[name].grpcStatus,
			httpStatus: errorDict.errorNameMap[name].httpStatus,
			message:    errorDict.errorNameMap[name].message,
			cause:      err.error,
			stack:      err.stack,
		}
	case error:
		return &custom{
			name:       name,
			errorCode:  errorDict.errorNameMap[name].errorCode,
			grpcStatus: errorDict.errorNameMap[name].grpcStatus,
			httpStatus: errorDict.errorNameMap[name].httpStatus,
			message:    errorDict.errorNameMap[name].message,
			cause:      err,
			stack:      callers(),
		}
	default:
		return &custom{
			name:       name,
			errorCode:  errorDict.errorNameMap[name].errorCode,
			grpcStatus: errorDict.errorNameMap[name].grpcStatus,
			httpStatus: errorDict.errorNameMap[name].httpStatus,
			message:    errorDict.errorNameMap[name].message,
			cause:      fmt.Errorf("%+v", err),
			stack:      callers(),
		}
	}
}

func NewCustomWithCode(code int, errInfo any) error {
	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()

	if _, ok := errorDict.errorCodeMap[code]; !ok {
		code = errorDict.errorNameMap["Err_Unknown"].errorCode
	}
	switch err := errInfo.(type) {
	case nil:
		return nil
	case *custom:
		return &custom{
			name:       errorDict.errorCodeMap[code].name,
			errorCode:  code,
			grpcStatus: errorDict.errorCodeMap[code].grpcStatus,
			httpStatus: errorDict.errorCodeMap[code].httpStatus,
			message:    errorDict.errorCodeMap[code].message,
			cause:      err.cause,
			stack:      err.stack,
		}
	case *withStack:
		return &custom{
			name:       errorDict.errorCodeMap[code].name,
			errorCode:  code,
			grpcStatus: errorDict.errorCodeMap[code].grpcStatus,
			httpStatus: errorDict.errorCodeMap[code].httpStatus,
			message:    errorDict.errorCodeMap[code].message,
			cause:      err.error,
			stack:      err.stack,
		}
	case error:
		return &custom{
			name:       errorDict.errorCodeMap[code].name,
			errorCode:  code,
			grpcStatus: errorDict.errorCodeMap[code].grpcStatus,
			httpStatus: errorDict.errorCodeMap[code].httpStatus,
			message:    errorDict.errorCodeMap[code].message,
			cause:      err,
			stack:      callers(),
		}
	default:
		return &custom{
			name:       errorDict.errorCodeMap[code].name,
			errorCode:  code,
			grpcStatus: errorDict.errorCodeMap[code].grpcStatus,
			httpStatus: errorDict.errorCodeMap[code].httpStatus,
			message:    errorDict.errorCodeMap[code].message,
			cause:      fmt.Errorf("%+v", err),
			stack:      callers(),
		}
	}
}

func ParseCustom(err error) Custom {
	if err == nil {
		return nil
	}
	if v, ok := err.(*custom); ok {
		return v
	}
	errorDict.mux.RLock()
	defer errorDict.mux.RUnlock()
	return errorDict.errorNameMap["Err_Unknown"]
}

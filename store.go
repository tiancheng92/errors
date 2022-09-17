package errors

import (
	"github.com/go-mysql-org/go-mysql/canal"
	canalLog "github.com/siddontang/go/log"
	"google.golang.org/grpc/codes"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"regexp"
	"time"
)

var (
	db *gorm.DB
)

type ErrorCode struct {
	ID         int64 `gorm:"primarykey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ErrorCode  int `gorm:"unique"`
	GrpcStatus codes.Code
	Name       string `gorm:"unique"`
	Message    string
}

func getAllErrorCode() []*ErrorCode {
	var res []*ErrorCode
	db.Find(&res)
	return res
}

func initDB(dsn string) {
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
		PrepareStmt:                              true,
	})
	if err != nil {
		log.Panicf("%+v", err)
	}

	if err = db.AutoMigrate(new(ErrorCode)); err != nil {
		log.Panicf("%+v", err)
	}
}

func getCanal(dsn string) *canal.Canal {
	params := regexp.MustCompile(
		`^(?:(?P<user>.*?)(?::(?P<passwd>.*))?@)?` +
			`(?:(?P<net>[^\(]*)(?:\((?P<addr>[^\)]*)\))?)?` +
			`\/(?P<dbname>.*?)` +
			`(?:\?(?P<params>[^\?]*))?$`).FindStringSubmatch(dsn)
	cfg := canal.NewDefaultConfig()
	cfg.User = params[1]
	cfg.Password = params[2]
	cfg.Addr = params[4]
	cfg.Dump.ExecutionPath = ""
	cfg.Logger.SetLevel(canalLog.LevelWarn)
	c, err := canal.NewCanal(cfg)
	if err != nil {
		log.Panicf("%+v", err)
	}
	return c
}

package errors

import (
	"github.com/go-mysql-org/go-mysql/canal"
	"log"
	"regexp"
)

func watch(dsn string) {
	cc := getCanal(dsn)

	cc.SetEventHandler(&handler{
		dsn: dsn,
	})

	pos, err := cc.GetMasterPos()
	if err != nil {
		log.Panicf("%+v", err)
	}

	err = cc.RunFrom(pos)
	if err != nil {
		log.Panicf("%+v", err)
	}
}

type handler struct {
	dsn string
	canal.DummyEventHandler
}

func (s *handler) OnRow(e *canal.RowsEvent) error {
	params := regexp.MustCompile(
		`^(?:(?P<user>.*?)(?::(?P<passwd>.*))?@)?` +
			`(?:(?P<net>[^\(]*)(?:\((?P<addr>[^\)]*)\))?)?` +
			`\/(?P<dbname>.*?)` +
			`(?:\?(?P<params>[^\?]*))?$`).FindStringSubmatch(s.dsn)
	if e.Table.Schema == params[5] && e.Table.Name == "error_codes" && (e.Action == "insert" || e.Action == "update" || e.Action == "delete") {
		register()
	}
	return nil
}

func (*handler) String() string {
	return "handler"
}

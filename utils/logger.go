package utils

import (
	"log"
	"strings"

	uuid "github.com/satori/go.uuid"
)

type LoggerId struct {
	Sid        string
	RequestSid string
}

func (l *LoggerId) SetSid() {
	session := uuid.NewV4()
	l.Sid = strings.ReplaceAll(session.String(), "-", "")
}

func (l *LoggerId) NewRequestSid() {
	l.RequestSid = strings.ReplaceAll(uuid.NewV4().String(), "-", "")
}

func (l *LoggerId) Log(message ...any) {
	log.Printf("Client:%s Request:%s - %v\n", l.Sid, l.RequestSid, message)
}

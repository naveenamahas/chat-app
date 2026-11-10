package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"os"
)

type Mailer interface {
	Send(to, subject, body string) error
}

// logMailer is used when SMTP_HOST is not set: it only prints the email.
// It lets you develop and test the queue without a real mail account.
type logMailer struct{}

func (logMailer) Send(to, subject, body string) error {
	log.Printf("[email] to=%s subject=%q", to, subject)
	return nil
}

type smtpMailer struct{ host, port, user, pass, from string }

func (m smtpMailer) Send(to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", m.from, to, subject, body)
	auth := smtp.PlainAuth("", m.user, m.pass, m.host)
	return smtp.SendMail(m.host+":"+m.port, auth, m.from, []string{to}, []byte(msg))
}

func newMailerFromEnv() Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return logMailer{}
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return smtpMailer{host: host, port: port, user: os.Getenv("SMTP_USER"), pass: os.Getenv("SMTP_PASS"), from: os.Getenv("SMTP_FROM")}
}

// registerJobHandlers connects job types to the code that runs them.
func registerJobHandlers(q *jobQueue, mailer Mailer) {
	q.Handle("welcome_email", func(ctx context.Context, j job) error {
		to := j.Payload["email"]
		if to == "" {
			return errors.New("welcome_email: missing email")
		}
		return mailer.Send(to, "Welcome to the chat!",
			"Hi "+j.Payload["username"]+",\n\nYour account is ready. Open the app and say hello.\n")
	})
}

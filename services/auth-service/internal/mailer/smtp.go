package mailer

import "net/smtp"

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// SMTPMailer = implementasi usecase.Mailer pakai net/smtp bawaan Go (tanpa library eksternal).
type SMTPMailer struct {
	cfg Config
}

func New(cfg Config) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) Send(to, subject, htmlBody string) error {
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	// Header dipisah "\r\n" (bukan cuma "\n"), baris kosong wajib jadi pemisah header/body.
	msg := "MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + htmlBody

	addr := m.cfg.Host + ":" + m.cfg.Port
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(msg))
}

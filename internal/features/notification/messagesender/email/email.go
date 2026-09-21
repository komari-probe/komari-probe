package email

import (
	"fmt"
	"net/mail"
	"strings"

	gomail "github.com/wneessen/go-mail"

	"github.com/komari-monitor/komari/internal/features/notification/messagesender/factory"
)

type EmailSender struct {
	Addition
}

func (e *EmailSender) GetName() string {
	return "email"
}

func (e *EmailSender) GetConfiguration() factory.Configuration {
	return &e.Addition
}

func (e *EmailSender) Init() error {
	return nil
}

func (e *EmailSender) Destroy() error {
	return nil
}

func (e *EmailSender) SendTextMessage(message, title string) error {
	if e.Addition.Host == "" || e.Addition.Sender == "" || e.Addition.Username == "" || e.Addition.Password == "" || e.Addition.Receiver == "" {
		return fmt.Errorf("email sending is not fully configured")
	}

	rcptList, err := parseRecipients(e.Addition.Receiver)
	if err != nil {
		return err
	}

	// PLAIN 是大多数现代 SMTP 服务器的默认方式；LOGIN 用于兼容微软邮箱、
	// 网易邮箱等使用非标准握手的服务器（无正式 RFC，遵循 IETF draft）。
	authType := gomail.SMTPAuthPlain
	if e.Addition.UseLoginAuth {
		authType = gomail.SMTPAuthLogin
	}

	opts := []gomail.Option{
		gomail.WithSMTPAuth(authType),
		gomail.WithUsername(e.Addition.Username),
		gomail.WithPassword(e.Addition.Password),
		gomail.WithPort(e.Addition.Port),
	}
	switch {
	case e.Addition.UseSSL && e.Addition.Port == 465:
		opts = append(opts, gomail.WithSSL()) // 隐式 TLS
	case e.Addition.UseSSL:
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory)) // STARTTLS，必须成功
	default:
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSOpportunistic)) // 尽力 STARTTLS，失败则明文
	}

	client, err := gomail.NewClient(e.Addition.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}

	msg := gomail.NewMsg()
	if err := msg.From(e.Addition.Sender); err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	if err := msg.To(rcptList...); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	msg.Subject(title)

	contentType := gomail.TypeTextPlain
	if looksLikeHTML(message) {
		contentType = gomail.TypeTextHTML
	}
	msg.SetBodyString(contentType, message)

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

// parseRecipients 解析逗号分隔的收件人列表；地址不符合 RFC 5322 时回退为按逗号
// 简单切分，兼容部分用户填入的不规范地址。
func parseRecipients(receiver string) ([]string, error) {
	if addrs, err := mail.ParseAddressList(receiver); err == nil {
		list := make([]string, 0, len(addrs))
		for _, a := range addrs {
			list = append(list, a.Address)
		}
		return list, nil
	}
	var list []string
	for _, p := range strings.Split(receiver, ",") {
		if p = strings.TrimSpace(p); p != "" {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no valid recipient address parsed")
	}
	return list, nil
}

// looksLikeHTML 粗略检测消息内容是否为 HTML，用于选择邮件正文的 Content-Type。
func looksLikeHTML(message string) bool {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<!doctype") ||
		(strings.Contains(trimmed, "<div") && strings.Contains(trimmed, "</div>"))
}

// 确保实现了 IMessageSender 接口
var _ factory.IMessageSender = (*EmailSender)(nil)

package message

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
)

func shouldAuth() bool {
	return config.SMTPAccount != "" || config.SMTPToken != ""
}

func SendEmail(subject string, receiver string, content string) error {
	if receiver == "" {
		return fmt.Errorf("receiver is empty")
	}
	if config.SMTPServer == "" {
		return fmt.Errorf("SMTPServer is empty")
	}
	if config.SMTPFrom == "" { // for compatibility
		config.SMTPFrom = config.SMTPAccount
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))

	// Extract domain from SMTPFrom
	parts := strings.Split(config.SMTPFrom, "@")
	var domain string
	if len(parts) > 1 {
		domain = parts[1]
	}
	// Generate a unique Message-ID
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return err
	}
	messageId := fmt.Sprintf("<%x@%s>", buf, domain)

	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s<%s>\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+ // add Message-ID header to avoid being treated as spam, RFC 5322
		"Date: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, config.SystemName, config.SMTPFrom, encodedSubject, messageId, time.Now().Format(time.RFC1123Z), content))

	auth := smtp.PlainAuth("", config.SMTPAccount, config.SMTPToken, config.SMTPServer)
	addr := fmt.Sprintf("%s:%d", config.SMTPServer, config.SMTPPort)
	to := strings.Split(receiver, ";")

	if config.SMTPPort == 465 || !shouldAuth() {
		// need advanced client
		var conn net.Conn
		var err error
		if config.SMTPPort == 465 {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         config.SMTPServer,
			}
			conn, err = tls.Dial("tcp", fmt.Sprintf("%s:%d", config.SMTPServer, config.SMTPPort), tlsConfig)
		} else {
			conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", config.SMTPServer, config.SMTPPort))
		}
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, config.SMTPServer)
		if err != nil {
			return err
		}
		defer client.Close()
		if shouldAuth() {
			if err = client.Auth(auth); err != nil {
				return err
			}
		}
		if err = client.Mail(config.SMTPFrom); err != nil {
			return err
		}
		receiverEmails := strings.Split(receiver, ";")
		for _, receiver := range receiverEmails {
			if err = client.Rcpt(receiver); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(mail)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}
	} else {
		err = smtp.SendMail(addr, auth, config.SMTPAccount, to, mail)
	}
	return err
}

// 异步不阻塞发送邮件, 增加锁避免发送重复
func SendMailASync(email string, subject string, content string) {
	go func() {
		if email != "" {
			if count, serr := common.RedisExists(fmt.Sprintf("send_mail:%s", random.StrToMd5(subject))); serr != nil || count == 0 {
				ok, err := common.RedisSetNx(fmt.Sprintf("send_mail:%s", random.StrToMd5(subject)), "1", time.Duration(60*time.Second))
				if ok || err == nil {
					err := SendEmail(subject, email, content)
					if err != nil {
						logger.SysErrorf("failed to send email: %s", err.Error())
					}
				}
			}
		}
	}()
}

// 发送邮件给管理员
// 异步不阻塞发送邮件, 增加锁避免发送重复
func SendMailToAdmin(subject string, content string) {
	go func() {
		if count, serr := common.RedisExists(fmt.Sprintf("send_mail:%s", random.StrToMd5(subject))); serr != nil || count == 0 {
			ok, err := common.RedisSetNx(fmt.Sprintf("send_mail:%s", random.StrToMd5(subject)), "1", time.Duration(60*time.Second))
			if ok || err == nil {
				if config.MessagePusherAddress != "" {
					err := SendMessage(subject, content, content)
					if err != nil {
						logger.SysError(fmt.Sprintf("failed to send message: %s", err.Error()))
					} else {
						return
					}
				}
				if config.RootUserEmail == "" {
					logger.SysError("failed to send email: RootUserEmail is empty")
					return
				}
				err := SendEmail(subject, config.RootUserEmail, content)
				if err != nil {
					logger.SysError(fmt.Sprintf("failed to send email: %s", err.Error()))
				}
			}
		}
	}()
}

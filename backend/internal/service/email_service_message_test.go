package service

import (
	"bytes"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildEmailMessageEncodesInternationalHeadersAndBody(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 40, 0, 0, time.UTC)
	message, err := buildEmailMessage(&SMTPConfig{
		From:     "zhibei@poke2api.com",
		FromName: "Poke 助手",
	}, "user@example.com", "[Poke API] 验证 QQ 账户绑定", `<p>您好，请打开 <a href="https://qqbot.mcwar.cn/bind?token=test">验证链接</a>。</p>`, now)
	require.NoError(t, err)
	require.Equal(t, "zhibei@poke2api.com", message.envelopeFrom)
	require.Equal(t, "user@example.com", message.envelopeTo)

	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
	require.NoError(t, err)

	from, err := mail.ParseAddress(parsed.Header.Get("From"))
	require.NoError(t, err)
	require.Equal(t, "Poke 助手", from.Name)
	require.Equal(t, "zhibei@poke2api.com", from.Address)

	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "[Poke API] 验证 QQ 账户绑定", decodedSubject)
	require.Equal(t, now.Format(time.RFC1123Z), parsed.Header.Get("Date"))
	require.Regexp(t, regexp.MustCompile(`^<[0-9]+\.[0-9a-f]{24}@poke2api\.com>$`), parsed.Header.Get("Message-ID"))
	require.Equal(t, "1.0", parsed.Header.Get("MIME-Version"))
	require.Equal(t, "quoted-printable", parsed.Header.Get("Content-Transfer-Encoding"))
	require.Equal(t, "auto-generated", parsed.Header.Get("Auto-Submitted"))

	decodedBody, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	require.NoError(t, err)
	require.Contains(t, string(decodedBody), "您好")
	require.Contains(t, string(decodedBody), "https://qqbot.mcwar.cn/bind?token=test")
}

func TestBuildEmailMessagePreventsHeaderInjection(t *testing.T) {
	message, err := buildEmailMessage(&SMTPConfig{
		From:     "zhibei@poke2api.com",
		FromName: "Poke API\r\nBcc: hidden@example.com",
	}, "user@example.com", "验证\r\nBcc: hidden@example.com", "<p>safe</p>", time.Now())
	require.NoError(t, err)

	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
	require.NoError(t, err)
	require.Empty(t, parsed.Header.Get("Bcc"))

	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	require.NoError(t, err)
	require.NotContains(t, decodedSubject, "\r")
	require.NotContains(t, decodedSubject, "\n")
}

func TestBuildEmailMessageRejectsInvalidMailbox(t *testing.T) {
	_, err := buildEmailMessage(&SMTPConfig{From: "not-an-email"}, "user@example.com", "subject", "body", time.Now())
	require.Error(t, err)

	_, err = buildEmailMessage(&SMTPConfig{From: "sender@example.com"}, "not-an-email", "subject", "body", time.Now())
	require.Error(t, err)
}

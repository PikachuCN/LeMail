package smtpserver

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
	"github.com/PikachuCN/LeMail/internal/smtpdebug"
)

type Server struct {
	manager *config.Manager
	store   *mailstore.Store
	hub     *realtime.Hub
	logger  *slog.Logger
	onMail  func(mailstore.Message)
	debug   *smtpdebug.Store
}

func New(manager *config.Manager, store *mailstore.Store, hub *realtime.Hub, logger *slog.Logger, onMail func(mailstore.Message)) *Server {
	return NewWithDebug(manager, store, hub, logger, onMail, nil)
}

func NewWithDebug(manager *config.Manager, store *mailstore.Store, hub *realtime.Hub, logger *slog.Logger, onMail func(mailstore.Message), debug *smtpdebug.Store) *Server {
	return &Server{manager: manager, store: store, hub: hub, logger: logger, onMail: onMail, debug: debug}
}

func (s *Server) ListenAndServe() error {
	cfg := s.manager.Get()
	server := smtp.NewServer(&backend{server: s})
	server.Addr = cfg.SMTP.Addr
	server.Domain = firstDomain(cfg)
	server.ReadTimeout = 10 * time.Second
	server.WriteTimeout = 10 * time.Second
	server.MaxMessageBytes = 10 << 20
	server.MaxRecipients = 100
	server.AllowInsecureAuth = true
	s.logger.Info("smtp server listening", "addr", server.Addr)
	s.record(smtpdebug.Event{Type: smtpdebug.EventListenStart, LocalAddr: server.Addr, Detail: "SMTP 服务开始监听"})
	network := server.Network
	if network == "" {
		network = "tcp"
	}
	listener, err := net.Listen(network, server.Addr)
	if err != nil {
		s.record(smtpdebug.Event{Type: smtpdebug.EventListenError, LocalAddr: server.Addr, Error: err.Error()})
		return err
	}
	return server.Serve(&debugListener{Listener: listener, server: s})
}

type backend struct {
	server *Server
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	sess := &session{
		server:     b.server,
		id:         nextSessionID(),
		remoteAddr: remoteAddress(c),
		localAddr:  localAddress(c),
		helo:       c.Hostname(),
	}
	sess.record(smtpdebug.Event{Type: smtpdebug.EventHelo, Helo: sess.helo, Detail: "SMTP 客户端已发送 HELO/EHLO"})
	return sess, nil
}

type session struct {
	server     *Server
	id         string
	remoteAddr string
	localAddr  string
	helo       string
	from       string
	recipients []string
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = strings.TrimSpace(from)
	s.recipients = nil
	s.record(smtpdebug.Event{Type: smtpdebug.EventMailFrom, From: s.from})
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	address, err := normalizeRecipient(to)
	if err != nil {
		return s.rejectRcpt(to, err)
	}
	cfg := s.server.manager.Get()
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" || domain == "" {
		return s.rejectRcpt(address, errors.New("invalid recipient address"))
	}
	if !cfg.HasDomain(domain) {
		return s.rejectRcpt(address, fmt.Errorf("domain %q is not accepted", domain))
	}
	if cfg.IsReservedLocalPart(local) {
		return s.rejectRcpt(address, fmt.Errorf("local part %q is reserved", local))
	}
	s.recipients = append(s.recipients, address)
	s.record(smtpdebug.Event{Type: smtpdebug.EventRcptAccept, To: address, Recipients: s.recipients})
	return nil
}

func (s *session) Data(r io.Reader) error {
	if len(s.recipients) == 0 {
		err := errors.New("no valid recipients")
		s.record(smtpdebug.Event{Type: smtpdebug.EventDataError, Error: err.Error()})
		return err
	}
	s.record(smtpdebug.Event{Type: smtpdebug.EventDataStart, From: s.from, Recipients: s.recipients})
	raw, err := io.ReadAll(io.LimitReader(r, 10<<20))
	if err != nil {
		s.record(smtpdebug.Event{Type: smtpdebug.EventDataError, Error: err.Error(), Recipients: s.recipients})
		return err
	}
	msg := mailstore.NewMessage(raw, s.from, s.recipients, time.Now())
	msg = s.server.store.Add(msg)
	if s.server.onMail != nil {
		s.server.onMail(msg)
	}
	for _, recipient := range msg.To {
		s.server.hub.Publish(recipient, msg)
	}
	s.server.logger.Info("mail received", "id", msg.ID, "to", strings.Join(msg.To, ","), "from", msg.From)
	s.record(smtpdebug.Event{
		Type:       smtpdebug.EventMailStored,
		From:       msg.From,
		Recipients: msg.To,
		MessageID:  msg.ID,
		Size:       int64(len(raw)),
		Detail:     "邮件已写入内存并完成实时推送",
	})
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *session) Logout() error { return nil }

func (s *session) rejectRcpt(to string, err error) error {
	s.record(smtpdebug.Event{Type: smtpdebug.EventRcptReject, To: strings.TrimSpace(to), Error: err.Error()})
	return err
}

func (s *session) record(event smtpdebug.Event) {
	if s == nil || s.server == nil {
		return
	}
	event.SessionID = s.id
	event.RemoteAddr = s.remoteAddr
	event.LocalAddr = s.localAddr
	event.Helo = firstNonEmpty(event.Helo, s.helo)
	event.From = firstNonEmpty(event.From, s.from)
	s.server.record(event)
}

func (s *Server) record(event smtpdebug.Event) {
	if s == nil || s.debug == nil {
		return
	}
	s.debug.Add(event)
}

type debugListener struct {
	net.Listener
	server *Server
}

func (l *debugListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.server != nil {
		l.server.record(smtpdebug.Event{
			Type:       smtpdebug.EventConnect,
			RemoteAddr: conn.RemoteAddr().String(),
			LocalAddr:  conn.LocalAddr().String(),
			Detail:     "TCP 连接已建立，等待 SMTP HELO/EHLO",
		})
	}
	return conn, nil
}

func remoteAddress(c *smtp.Conn) string {
	if c == nil || c.Conn() == nil || c.Conn().RemoteAddr() == nil {
		return ""
	}
	return c.Conn().RemoteAddr().String()
}

func localAddress(c *smtp.Conn) string {
	if c == nil || c.Conn() == nil || c.Conn().LocalAddr() == nil {
		return ""
	}
	return c.Conn().LocalAddr().String()
}

func nextSessionID() string {
	return "smtp_session_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeRecipient(address string) (string, error) {
	address = strings.ToLower(strings.TrimSpace(strings.Trim(address, "<>")))
	if !strings.Contains(address, "@") {
		return "", errors.New("invalid recipient address")
	}
	return address, nil
}

func firstDomain(cfg config.Config) string {
	if len(cfg.Mail.Domains) > 0 {
		return cfg.Mail.Domains[0]
	}
	return "localhost"
}

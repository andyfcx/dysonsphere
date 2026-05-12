// Package observer provides external observation of cron jobs.
package observer

import (
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// CronMailMessage is a parsed cron output message from a Unix mbox spool file.
type CronMailMessage struct {
	EnvelopeDate time.Time
	Subject      string
	Command      string // extracted from "Cron <user@host> command"
	MailUser     string // the "<user@host>" portion
	Body         string // stdout+stderr captured by cron
	MessageHash  string // md5(envelope line + subject) for dedup
}

// MailSpoolReader reads cron output from Unix mbox files (e.g. /var/mail/root).
// It is stateless; callers supply and store the byte offset between calls.
type MailSpoolReader struct{}

// ReadNew returns unread cron mail messages from path, starting at fromOffset.
// Returns the parsed messages and the new offset to pass on the next call.
// If the file does not exist, returns (nil, 0, nil) silently.
// If the file was truncated (mailbox cleared), restarts from offset 0.
func (r *MailSpoolReader) ReadNew(path string, fromOffset int64) ([]*CronMailMessage, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, fromOffset, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fromOffset, err
	}
	size := fi.Size()
	if size == 0 {
		return nil, 0, nil
	}
	// Mailbox was cleared since last read.
	if fromOffset > size {
		slog.Debug("mail spool truncated, resetting offset", "path", path)
		fromOffset = 0
	}
	if fromOffset == size {
		return nil, fromOffset, nil
	}

	if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
		return nil, fromOffset, err
	}

	data := make([]byte, size-fromOffset)
	n, err := io.ReadFull(f, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fromOffset, fmt.Errorf("read %s: %w", path, err)
	}
	data = data[:n]

	messages := parseMbox(data)
	return messages, fromOffset + int64(n), nil
}

// ─── mbox parsing ────────────────────────────────────────────────────────────

func parseMbox(data []byte) []*CronMailMessage {
	var messages []*CronMailMessage
	var current []string
	inMessage := false

	for _, line := range strings.Split(string(data), "\n") {
		if isMboxFromLine(line) {
			if inMessage && len(current) > 0 {
				if msg := buildMessage(current); msg != nil {
					messages = append(messages, msg)
				}
			}
			current = []string{line}
			inMessage = true
		} else if inMessage {
			current = append(current, line)
		}
	}
	if inMessage && len(current) > 0 {
		if msg := buildMessage(current); msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

// isMboxFromLine returns true for the "From " envelope separator line.
// Standard format: "From sender weekday mon day hh:mm:ss year"
func isMboxFromLine(line string) bool {
	if !strings.HasPrefix(line, "From ") {
		return false
	}
	fields := strings.Fields(line)
	return len(fields) >= 3
}

// buildMessage parses a slice of raw lines (starting with the From_ envelope)
// into a CronMailMessage. Returns nil if the message is not a cron output mail.
func buildMessage(lines []string) *CronMailMessage {
	if len(lines) == 0 {
		return nil
	}

	envelopeDate := parseEnvelopeDate(lines[0])

	headers := map[string]string{}
	bodyStartIdx := len(lines)
	inHeaders := true
	for i := 1; i < len(lines); i++ {
		if inHeaders {
			if lines[i] == "" {
				bodyStartIdx = i + 1
				inHeaders = false
				continue
			}
			// Continuation lines start with whitespace.
			if (lines[i][0] == ' ' || lines[i][0] == '\t') {
				// fold into the last header (skip for simplicity)
				continue
			}
			if idx := strings.Index(lines[i], ":"); idx > 0 {
				key := strings.ToLower(strings.TrimSpace(lines[i][:idx]))
				val := strings.TrimSpace(lines[i][idx+1:])
				headers[key] = val
			}
		}
	}

	subject := headers["subject"]
	if subject == "" {
		return nil
	}

	command, mailUser, ok := parseCronSubject(subject)
	if !ok {
		return nil
	}

	// Collect body; strip trailing blank lines.
	bodyLines := lines[bodyStartIdx:]
	for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
		bodyLines = bodyLines[:len(bodyLines)-1]
	}
	// Unescape mbox-quoted ">From " back to "From " in body.
	for i, bl := range bodyLines {
		if strings.HasPrefix(bl, ">From ") {
			bodyLines[i] = bl[1:]
		}
	}
	body := strings.Join(bodyLines, "\n")

	h := md5.Sum([]byte(lines[0] + subject))
	msgHash := fmt.Sprintf("%x", h)

	return &CronMailMessage{
		EnvelopeDate: envelopeDate,
		Subject:      subject,
		Command:      command,
		MailUser:     mailUser,
		Body:         body,
		MessageHash:  msgHash,
	}
}

// parseCronSubject extracts the command and mail-user from a cron Subject line.
// Accepted formats:
//
//	Cron <root@host> /usr/bin/command args
//	Cron Daemon
func parseCronSubject(subject string) (command, mailUser string, ok bool) {
	// Require "Cron " prefix (case-insensitive).
	if !strings.HasPrefix(strings.ToUpper(subject), "CRON ") {
		return "", "", false
	}
	rest := strings.TrimSpace(subject[5:])
	if !strings.HasPrefix(rest, "<") {
		return "", "", false
	}
	end := strings.Index(rest, ">")
	if end < 0 {
		return "", "", false
	}
	mailUser = rest[1:end]
	command = strings.TrimSpace(rest[end+1:])
	if command == "" {
		return "", "", false
	}
	return command, mailUser, true
}

// envelopeDateFormats are tried in order when parsing the From_ date.
var envelopeDateFormats = []string{
	"Mon Jan _2 15:04:05 2006",
	"Mon Jan  2 15:04:05 2006",
	"Mon Jan 02 15:04:05 2006",
	"Mon Jan _2 15:04:05 MST 2006",
	"Mon Jan  2 15:04:05 MST 2006",
	"Mon Jan 02 15:04:05 MST 2006",
}

func parseEnvelopeDate(fromLine string) time.Time {
	// "From sender weekday mon day time [tz] year"
	// Skip "From" and the sender token.
	fields := strings.Fields(fromLine)
	if len(fields) < 6 {
		return time.Now()
	}
	dateStr := strings.Join(fields[2:], " ")
	for _, format := range envelopeDateFormats {
		t, err := time.Parse(format, dateStr)
		if err == nil {
			return t
		}
	}
	return time.Now()
}

package smtp4dev

import (
	"time"

	"github.com/google/uuid"
)

type MessageResult struct {
	IsRelayed       bool      `json:"isRelayed"`
	DeliveredTo     string    `json:"deliveredTo"`
	ID              uuid.UUID `json:"id"`
	From            string    `json:"from"`
	To              []string  `json:"to"`
	ReceivedDate    time.Time `json:"receivedDate"`
	Subject         string    `json:"subject"`
	AttachmentCount int       `json:"attachmentCount"`
	IsUnread        bool      `json:"isUnread"`
	HasWarnings     bool      `json:"hasWarnings"`
}

type MessagePage struct {
	CurrentPage    int             `json:"currentPage"`
	PageCount      int             `json:"pageCount"`
	PageSize       int             `json:"pageSize"`
	RowCount       int             `json:"rowCount"`
	FirstRowOnPage int             `json:"firstRowOnPage"`
	LastRowOnPage  int             `json:"lastRowOnPage"`
	Results        []MessageResult `json:"results"`
}

type Message struct {
	SessionEncoding   string           `json:"sessionEncoding"`
	EightBitTransport bool             `json:"eightBitTransport"`
	HasHtmlBody       bool             `json:"hasHtmlBody"`
	HasPlainTextBody  bool             `json:"hasPlainTextBody"`
	ID                uuid.UUID        `json:"id"`
	From              string           `json:"from"`
	To                []string         `json:"to"`
	Cc                []string         `json:"cc"`
	Bcc               []string         `json:"bcc"`
	DeliveredTo       []string         `json:"deliveredTo"`
	ReceivedDate      time.Time        `json:"receivedDate"`
	SecureConnection  bool             `json:"secureConnection"`
	Subject           string           `json:"subject"`
	Parts             []MessageEntity  `json:"parts"`
	Headers           []Header         `json:"headers"`
	MimeParseError    string           `json:"mimeParseError"`
	RelayError        string           `json:"relayError"`
	Warnings          []MessageWarning `json:"warnings"`
	Data              string           `json:"data"`
}

type MessageEntity struct {
	ID           string           `json:"id"`
	Headers      []Header         `json:"headers"`
	ChildParts   []string         `json:"childParts"`
	Name         string           `json:"name"`
	MessageID    uuid.UUID        `json:"messageId"`
	ContentID    string           `json:"contentId"`
	Attachments  []Attachment     `json:"attachments"`
	Warnings     []MessageWarning `json:"warnings"`
	Size         int              `json:"size"`
	IsAttachment bool             `json:"isAttachment"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Attachment struct {
	FileName  string `json:"fileName"`
	ContentID string `json:"contentId"`
	ID        string `json:"id"`
	URL       string `json:"url"`
}

type MessageWarning struct {
	Details string `json:"details"`
}

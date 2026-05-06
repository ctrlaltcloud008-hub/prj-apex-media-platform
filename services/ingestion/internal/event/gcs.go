package event

import (
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub/v2"
)

const EventObjectFinalize = "OBJECT_FINALIZE"

type Attributes struct {
	NotificationConfig string `json:"notificationConfig"`
	EventType          string `json:"eventType"`
	PayloadFormat      string `json:"payloadFormat"`
	BucketID           string `json:"bucketId"`
	ObjectID           string `json:"objectId"`
	ObjectGeneration   string `json:"objectGeneration"`
	EventTime          string `json:"eventTime"`
}

type GCSObjectNotification struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Bucket             string `json:"bucket"`
	Generation         int64  `json:"generation,string"`
	ContentType        string `json:"contentType"`
	Size               int64  `json:"size,string"`
	Md5Hash            string `json:"md5Hash"`
	ContentEncoding    string `json:"contentEncoding"`
	ContentDisposition string `json:"contentDisposition"`
	ContentLanguage    string `json:"contentLanguage"`
	Crc32c             string `json:"crc32c"`
	Etag               string `json:"etag"`
	TimeCreated        string `json:"timeCreated"`
}

type Message struct {
	MessageID    string
	PublishTime  time.Time
	Attributes   Attributes
	Notification GCSObjectNotification
}

func ParseGCSFinalizeMessage(msg *pubsub.Message) (*Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}

	attrs := Attributes{
		NotificationConfig: msg.Attributes["notificationConfig"],
		EventType:          msg.Attributes["eventType"],
		PayloadFormat:      msg.Attributes["payloadFormat"],
		BucketID:           msg.Attributes["bucketId"],
		ObjectID:           msg.Attributes["objectId"],
		ObjectGeneration:   msg.Attributes["objectGeneration"],
		EventTime:          msg.Attributes["eventTime"],
	}

	if attrs.EventType != EventObjectFinalize {
		return nil, fmt.Errorf("unsupported event type: %s", attrs.EventType)
	}

	var notification GCSObjectNotification
	if err := json.Unmarshal(msg.Data, &notification); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification data %q: %w", string(msg.Data), err)
	}

	return &Message{
		MessageID:    msg.ID,
		PublishTime:  msg.PublishTime,
		Attributes:   attrs,
		Notification: notification,
	}, nil
}

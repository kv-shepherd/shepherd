package provider

import notificationcontract "kv-shepherd.io/shepherd/internal/provider/notificationcontract"

// NotificationProvider defines the notification interface.
type NotificationProvider = notificationcontract.NotificationProvider

// Notification represents a notification message.
type Notification = notificationcontract.Notification

// Package email provides transactional email delivery via the Resend API.
// It wraps the resend-go SDK and exposes typed methods for each email event
// used by the KosKu platform.
package email

import (
	"fmt"

	"github.com/resend/resend-go/v2"
)

// Client wraps the Resend SDK client and holds the configured sender address.
type Client struct {
	resend *resend.Client
	from   string
}

// New creates a new email Client using the provided Resend API key.
// The from address defaults to "KosKu <noreply@kosku.id>".
func New(apiKey string) *Client {
	return &Client{
		resend: resend.NewClient(apiKey),
		from:   "KosKu <noreply@kosku.id>",
	}
}

// SendInvitation sends a tenant invitation email containing the unique invite
// link. recipientEmail is the tenant's email address, ownerName is the name of
// the owner sending the invite, and inviteURL is the full invitation link.
func (c *Client) SendInvitation(recipientEmail, ownerName, inviteURL string) error {
	subject := fmt.Sprintf("%s has invited you to KosKu", ownerName)
	html := fmt.Sprintf(`
<h2>You've been invited to KosKu</h2>
<p>%s has invited you to join their property on KosKu.</p>
<p>Click the link below to accept the invitation and complete your registration:</p>
<p><a href="%s">Accept Invitation</a></p>
<p>This link expires in 7 days.</p>
<p>If you did not expect this invitation, you can safely ignore this email.</p>
`, ownerName, inviteURL)

	return c.send(recipientEmail, subject, html)
}

// SendRegistrationApproved notifies a tenant that their registration has been
// approved by the owner. appURL is the URL to the tenant portal.
func (c *Client) SendRegistrationApproved(recipientEmail, tenantName, appURL string) error {
	subject := "Your KosKu registration has been approved"
	html := fmt.Sprintf(`
<h2>Registration Approved</h2>
<p>Hi %s,</p>
<p>Great news! Your registration on KosKu has been approved. You can now log in to your tenant portal.</p>
<p><a href="%s">Go to Tenant Portal</a></p>
`, tenantName, appURL)

	return c.send(recipientEmail, subject, html)
}

// SendRegistrationRejected notifies a tenant that their registration has been
// rejected by the owner.
func (c *Client) SendRegistrationRejected(recipientEmail, tenantName string) error {
	subject := "Your KosKu registration was not approved"
	html := fmt.Sprintf(`
<h2>Registration Not Approved</h2>
<p>Hi %s,</p>
<p>We're sorry to inform you that your registration on KosKu was not approved at this time.</p>
<p>If you believe this is a mistake, please contact the property owner directly.</p>
`, tenantName)

	return c.send(recipientEmail, subject, html)
}

// SendPendingRegistrationNotice notifies an owner that a new user has
// self-registered and is awaiting approval.
func (c *Client) SendPendingRegistrationNotice(ownerEmail, ownerName, tenantName, dashboardURL string) error {
	subject := fmt.Sprintf("New registration pending approval: %s", tenantName)
	html := fmt.Sprintf(`
<h2>New Registration Pending Approval</h2>
<p>Hi %s,</p>
<p>A new user, <strong>%s</strong>, has registered on KosKu and is awaiting your approval.</p>
<p><a href="%s">Review Registration</a></p>
`, ownerName, tenantName, dashboardURL)

	return c.send(ownerEmail, subject, html)
}

// SendContractExpiryReminder notifies a recipient that a contract is expiring soon.
// recipientEmail is the email address to notify, recipientName is their display name,
// tenantName is the tenant's name, roomNumber is the room number, propertyName is the
// property name, and endDate is the contract end date string.
func (c *Client) SendContractExpiryReminder(recipientEmail, recipientName, tenantName, roomNumber, propertyName, endDate string) error {
	subject := fmt.Sprintf("Contract expiring soon: %s - Room %s", propertyName, roomNumber)
	html := fmt.Sprintf(`
<h2>Contract Expiry Reminder</h2>
<p>Hi %s,</p>
<p>This is a reminder that the rental contract for <strong>%s</strong> in room <strong>%s</strong> at <strong>%s</strong> is expiring on <strong>%s</strong>.</p>
<p>Please take action before the contract expires.</p>
`, recipientName, tenantName, roomNumber, propertyName, endDate)

	return c.send(recipientEmail, subject, html)
}

// send is the internal helper that dispatches a single email via the Resend API.
func (c *Client) send(to, subject, html string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
	}

	_, err := c.resend.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("email: failed to send to %s: %w", to, err)
	}
	return nil
}

// SendPaymentConfirmed notifies a tenant that their payment has been confirmed.
func (c *Client) SendPaymentConfirmed(recipientEmail, tenantName, propertyName, amount, periodMonth, periodYear string) error {
	subject := fmt.Sprintf("Payment confirmed — %s %s/%s", propertyName, periodMonth, periodYear)
	html := fmt.Sprintf(`
<h2>Payment Confirmed</h2>
<p>Hi %s,</p>
<p>Your payment of <strong>%s</strong> for <strong>%s</strong> (%s/%s) has been confirmed.</p>
<p>Thank you for your payment.</p>
`, tenantName, amount, propertyName, periodMonth, periodYear)

	return c.send(recipientEmail, subject, html)
}

// SendPaymentRejected notifies a tenant that their payment has been rejected.
func (c *Client) SendPaymentRejected(recipientEmail, tenantName, propertyName, reason string) error {
	subject := fmt.Sprintf("Payment rejected — %s", propertyName)
	html := fmt.Sprintf(`
<h2>Payment Rejected</h2>
<p>Hi %s,</p>
<p>Unfortunately, your payment for <strong>%s</strong> has been rejected.</p>
<p><strong>Reason:</strong> %s</p>
<p>Please resubmit your payment proof or contact the property owner for assistance.</p>
`, tenantName, propertyName, reason)

	return c.send(recipientEmail, subject, html)
}

// SendComplaintSubmitted notifies an owner that a new complaint ticket has been submitted.
func (c *Client) SendComplaintSubmitted(ownerEmail, ownerName, propertyName, ticketID, ticketTitle string) error {
	subject := fmt.Sprintf("New complaint ticket — %s", propertyName)
	html := fmt.Sprintf(`
<h2>New Complaint Ticket</h2>
<p>Hi %s,</p>
<p>A new complaint ticket has been submitted for <strong>%s</strong>.</p>
<p><strong>Ticket ID:</strong> %s</p>
<p><strong>Title:</strong> %s</p>
<p>Please log in to your dashboard to review and respond to this complaint.</p>
`, ownerName, propertyName, ticketID, ticketTitle)

	return c.send(ownerEmail, subject, html)
}

// SendComplaintUpdated notifies a tenant that their complaint ticket status has been updated.
func (c *Client) SendComplaintUpdated(tenantEmail, tenantName, propertyName, ticketID, newStatus string) error {
	subject := fmt.Sprintf("Complaint ticket updated — %s", propertyName)
	html := fmt.Sprintf(`
<h2>Complaint Ticket Updated</h2>
<p>Hi %s,</p>
<p>Your complaint ticket for <strong>%s</strong> has been updated.</p>
<p><strong>Ticket ID:</strong> %s</p>
<p><strong>New Status:</strong> %s</p>
<p>Please log in to your tenant portal to view the full details.</p>
`, tenantName, propertyName, ticketID, newStatus)

	return c.send(tenantEmail, subject, html)
}

// SendNewMessageNotification notifies a recipient that they have unread messages
// from a sender that are older than 30 minutes.
func (c *Client) SendNewMessageNotification(recipientEmail, recipientName, senderName string) error {
	subject := fmt.Sprintf("You have unread messages from %s", senderName)
	html := fmt.Sprintf(`
<h2>Unread Messages</h2>
<p>Hi %s,</p>
<p>You have unread messages from <strong>%s</strong> that have been waiting for more than 30 minutes.</p>
<p>Please log in to your KosKu portal to read and reply to your messages.</p>
`, recipientName, senderName)

	return c.send(recipientEmail, subject, html)
}

// SendStaffInvitation sends an invitation email to a new staff member.
// recipientEmail is the staff member's email, ownerName is the owner's name,
// and inviteURL is the link to accept the invitation.
func (c *Client) SendStaffInvitation(recipientEmail, ownerName, inviteURL string) error {
	subject := fmt.Sprintf("%s has invited you as staff on KosKu", ownerName)
	html := fmt.Sprintf(`
<h2>You've been invited as staff on KosKu</h2>
<p>%s has invited you to join their property management team on KosKu.</p>
<p>Click the link below to accept the invitation and complete your registration:</p>
<p><a href="%s">Accept Staff Invitation</a></p>
<p>This link expires in 7 days.</p>
<p>If you did not expect this invitation, you can safely ignore this email.</p>
`, ownerName, inviteURL)

	return c.send(recipientEmail, subject, html)
}

// SendAnnouncement sends an announcement email to a tenant.
// recipientEmail is the tenant's email address (may be empty if not available),
// recipientName is the tenant's display name, title is the announcement title,
// and body is the announcement body text.
func (c *Client) SendAnnouncement(recipientEmail, recipientName, title, body string) error {
	subject := fmt.Sprintf("Announcement: %s", title)
	html := fmt.Sprintf(`
<h2>%s</h2>
<p>Hi %s,</p>
<p>%s</p>
<p>Please log in to your KosKu portal for more details.</p>
`, title, recipientName, body)

	return c.send(recipientEmail, subject, html)
}

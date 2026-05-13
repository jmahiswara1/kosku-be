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

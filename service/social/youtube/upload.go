package youtube

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// Publish implements social.Connector (task 5 of epic 18).
func (c *Connector) Publish(context.Context, connections.Credential, social.Upload) (social.Published, error) {
	return social.Published{}, errors.New("youtube: the upload is not built yet")
}

// Status implements social.Connector (task 5 of epic 18).
func (c *Connector) Status(context.Context, connections.Credential, string) (social.Remote, error) {
	return social.Remote{}, errors.New("youtube: the status is not built yet")
}

package sql

import (
	"database/sql"

	"github.com/pkg/errors"
	"github.com/urandom/readeef/content"
	"github.com/urandom/readeef/content/repo/sql/db"
	"github.com/urandom/readeef/log"
)

type subscriptionRepo struct {
	db *db.DB

	log log.Log
}

func (r subscriptionRepo) Get(feed content.Feed) (content.Subscription, error) {
	if err := feed.Validate(); err != nil {
		return content.Subscription{}, errors.WithMessage(err, "validating feed")
	}
	r.log.Infoln("Getting feed subscription")

	var subscription content.Subscription
	err := r.db.Get(&subscription, r.db.SQL().Subscription.GetForFeed, feed.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrNoContent
		}

		return content.Subscription{}, errors.Wrapf(err, "getting subscription for feed %s", feed)
	}

	subscription.FeedID = feed.ID

	return subscription, nil
}

func (r subscriptionRepo) All() ([]content.Subscription, error) {
	r.log.Infoln("Getting all subscriptions")

	var subscriptions []content.Subscription
	err := r.db.Select(&subscriptions, r.db.SQL().Subscription.All)
	if err != nil {
		return []content.Subscription{}, errors.Wrap(err, "getting hubbub subscriptions")
	}

	return subscriptions, nil
}

func (r subscriptionRepo) Update(subscription content.Subscription) error {
	if err := subscription.Validate(); err != nil {
		return errors.WithMessage(err, "validating subscription")
	}

	r.log.Infof("Updating subscription %s", subscription)

	tx, err := r.db.Beginx()
	if err != nil {
		return errors.Wrap(err, "creating transaction")
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamed(r.db.SQL().Subscription.Update)
	if err != nil {
		return errors.Wrap(err, "preparing subscription update stmt")
	}
	defer stmt.Close()

	res, err := stmt.Exec(subscription)
	if err != nil {
		return errors.Wrap(err, "executing subscription update stmt")
	}

	if num, err := res.RowsAffected(); err == nil && num > 0 {
		if err := tx.Commit(); err != nil {
			return errors.Wrap(err, "committing transaction")
		}

		return nil
	}

	stmt, err = tx.PrepareNamed(r.db.SQL().Subscription.Create)
	if err != nil {
		return errors.Wrap(err, "preparing subscription create stmt")
	}
	defer stmt.Close()

	_, err = stmt.Exec(subscription)
	if err != nil {
		return errors.Wrap(err, "executing subscription create stmt")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "committing transaction")
	}

	return nil

}

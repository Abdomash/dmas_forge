package workflow

import (
	"context"

	"github.com/blueprint-uservices/blueprint/runtime/core/backend"
	"go.mongodb.org/mongo-driver/bson"
)

type RateServiceImpl struct {
	rateDB backend.NoSQLDatabase
}

func NewRateServiceImpl(ctx context.Context, rateDB backend.NoSQLDatabase) (RateService, error) {
	if err := initRateDB(ctx, rateDB); err != nil {
		return nil, err
	}

	return &RateServiceImpl{rateDB: rateDB}, nil
}

func initRateDB(ctx context.Context, db backend.NoSQLDatabase) error {
	c, err := db.GetCollection(ctx, "rate-db", "inventory")
	if err != nil {
		return err
	}

	if err = c.DeleteMany(ctx, bson.D{}); err != nil {
		return err
	}

	for _, r := range seedRates() {
		if err = c.InsertOne(ctx, &r); err != nil {
			return err
		}
	}

	return nil
}

func (r *RateServiceImpl) GetRates(ctx context.Context, hotelIDs []string) ([]RatePlan, error) {
	plans := []RatePlan{}
	c, err := r.rateDB.GetCollection(ctx, "rate-db", "inventory")
	if err != nil {
		return nil, err
	}

	for _, id := range hotelIDs {
		var hotelPlans []RatePlan
		cur, err := c.FindMany(ctx, bson.D{{"hotelid", id}})
		if err != nil {
			return plans, err
		}

		if err = cur.All(ctx, &hotelPlans); err != nil {
			return plans, err
		}

		plans = append(plans, hotelPlans...)
	}

	return plans, nil
}

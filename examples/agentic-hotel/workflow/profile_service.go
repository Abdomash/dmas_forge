package workflow

import (
	"context"

	"github.com/blueprint-uservices/blueprint/runtime/core/backend"
	"go.mongodb.org/mongo-driver/bson"
)

type ProfileServiceImpl struct {
	profileDB backend.NoSQLDatabase
}

func NewProfileServiceImpl(ctx context.Context, profileDB backend.NoSQLDatabase) (ProfileService, error) {
	if err := initProfileDB(ctx, profileDB); err != nil {
		return nil, err
	}

	return &ProfileServiceImpl{profileDB: profileDB}, nil
}

func initProfileDB(ctx context.Context, db backend.NoSQLDatabase) error {
	c, err := db.GetCollection(ctx, "profile-db", "hotels")
	if err != nil {
		return err
	}

	if err = c.DeleteMany(ctx, bson.D{}); err != nil {
		return err
	}

	for _, p := range seedProfiles() {
		if err = c.InsertOne(ctx, &p); err != nil {
			return err
		}
	}

	return nil
}

func (p *ProfileServiceImpl) GetProfiles(ctx context.Context, hotelIDs []string, locale string) ([]HotelProfile, error) {
	profiles := []HotelProfile{}
	c, err := p.profileDB.GetCollection(ctx, "profile-db", "hotels")
	if err != nil {
		return nil, err
	}

	for _, id := range hotelIDs {
		var profile HotelProfile
		cur, err := c.FindOne(ctx, bson.D{{"id", id}})
		if err != nil {
			return profiles, err
		}

		ok, err := cur.One(ctx, &profile)
		if err != nil {
			return profiles, err
		}

		if ok {
			profiles = append(profiles, profile)
		}
	}

	return profiles, nil
}

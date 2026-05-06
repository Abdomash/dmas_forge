package workflow

import (
	"context"

	"github.com/blueprint-uservices/blueprint/runtime/core/backend"
	geoindex "github.com/hailocab/go-geoindex"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	MAXSEARCHRESULTS = 5
	MAXSEARCHRADIUS  = 10
)

type GeoServiceImpl struct {
	geoDB backend.NoSQLDatabase
	index *geoindex.ClusteringIndex
}

func NewGeoServiceImpl(ctx context.Context, geoDB backend.NoSQLDatabase) (GeoService, error) {
	if err := initGeoDB(ctx, geoDB); err != nil {
		return nil, err
	}

	s := &GeoServiceImpl{geoDB: geoDB}
	return s, s.newGeoIndex(ctx)
}

func initGeoDB(ctx context.Context, db backend.NoSQLDatabase) error {
	c, err := db.GetCollection(ctx, "geo-db", "geo")
	if err != nil {
		return err
	}

	if err = c.DeleteMany(ctx, bson.D{}); err != nil {
		return err
	}

	for _, p := range seedPoints() {
		if err = c.InsertOne(ctx, &p); err != nil {
			return err
		}
	}

	return nil
}

func (g *GeoServiceImpl) newGeoIndex(ctx context.Context) error {
	c, err := g.geoDB.GetCollection(ctx, "geo-db", "geo")
	if err != nil {
		return err
	}

	var points []Point
	cur, err := c.FindMany(ctx, bson.D{})
	if err != nil {
		return err
	}

	if err = cur.All(ctx, &points); err != nil {
		return err
	}

	g.index = geoindex.NewClusteringIndex()
	for _, p := range points {
		g.index.Add(p)
	}

	return nil
}

func (g *GeoServiceImpl) Nearby(ctx context.Context, lat float64, lon float64) ([]string, error) {
	center := &Point{Plat: lat, Plon: lon}
	pts := g.index.KNearest(
		center,
		MAXSEARCHRESULTS,
		geoindex.Km(MAXSEARCHRADIUS),
		func(p geoindex.Point) bool { return true },
	)

	ids := make([]string, 0, len(pts))
	for _, p := range pts {
		ids = append(ids, p.Id())
	}

	return ids, nil
}

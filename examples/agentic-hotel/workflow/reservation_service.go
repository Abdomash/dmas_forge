package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/blueprint-uservices/blueprint/runtime/core/backend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ReservationServiceImpl struct {
	reserveDB backend.NoSQLDatabase
}

func NewReservationServiceImpl(ctx context.Context, reserveDB backend.NoSQLDatabase) (ReservationService, error) {
	if err := initReservationDB(ctx, reserveDB); err != nil {
		return nil, err
	}

	return &ReservationServiceImpl{reserveDB: reserveDB}, nil
}

func initReservationDB(ctx context.Context, db backend.NoSQLDatabase) error {
	rc, err := db.GetCollection(ctx, "reservation-db", "reservation")
	if err != nil {
		return err
	}

	if err = rc.DeleteMany(ctx, bson.D{}); err != nil {
		return err
	}

	seedReservation := Reservation{
		HotelID:      "4",
		CustomerName: "Alice",
		InDate:       "2015-04-09",
		OutDate:      "2015-04-10",
		Number:       1,
	}
	if err = rc.InsertOne(ctx, &seedReservation); err != nil {
		return err
	}

	nc, err := db.GetCollection(ctx, "reservation-db", "number")
	if err != nil {
		return err
	}

	if err = nc.DeleteMany(ctx, bson.D{}); err != nil {
		return err
	}

	for _, hotelNumber := range seedHotelNumbers() {
		if err = nc.InsertOne(ctx, &hotelNumber); err != nil {
			return err
		}
	}

	bc, err := db.GetCollection(ctx, "reservation-db", "booking")
	if err != nil {
		return err
	}

	return bc.DeleteMany(ctx, bson.D{})
}

func (r *ReservationServiceImpl) CheckAvailability(ctx context.Context, customerName string, hotelIDs []string, inDate string, outDate string, roomCount int64) ([]string, error) {
	rc, nc, err := r.collections(ctx)
	if err != nil {
		return nil, err
	}

	available := []string{}
	for _, hotelID := range hotelIDs {
		ok, err := availableForHotel(ctx, rc, nc, hotelID, inDate, outDate, roomCount)
		if err != nil {
			return available, err
		}

		if ok {
			available = append(available, hotelID)
		}
	}

	return available, nil
}

func (r *ReservationServiceImpl) MakeReservation(ctx context.Context, customerName string, hotelIDs []string, inDate string, outDate string, roomCount int64) (BookingResult, error) {
	if len(hotelIDs) == 0 {
		return BookingResult{Status: "invalid_request", Message: "hotel_id is required"}, nil
	}

	rc, nc, err := r.collections(ctx)
	if err != nil {
		return BookingResult{}, err
	}

	hotelID := hotelIDs[0]
	ok, err := availableForHotel(ctx, rc, nc, hotelID, inDate, outDate, roomCount)
	if err != nil {
		return BookingResult{}, err
	}

	if !ok {
		return BookingResult{Status: "unavailable", Message: "hotel is unavailable", HotelID: hotelID}, nil
	}

	if err = insertReservationDays(ctx, rc, hotelID, customerName, inDate, outDate, roomCount); err != nil {
		return BookingResult{}, err
	}

	bookingID := primitive.NewObjectID().Hex()
	booking := Booking{
		BookingID:    bookingID,
		HotelID:      hotelID,
		CustomerName: customerName,
		InDate:       inDate,
		OutDate:      outDate,
		RoomCount:    roomCount,
		Status:       "confirmed",
	}

	bc, err := r.reserveDB.GetCollection(ctx, "reservation-db", "booking")
	if err != nil {
		return BookingResult{}, err
	}

	if err = bc.InsertOne(ctx, &booking); err != nil {
		return BookingResult{}, err
	}

	return BookingResult{Status: "confirmed", BookingID: bookingID, HotelID: hotelID}, nil
}

func (r *ReservationServiceImpl) GetBooking(ctx context.Context, bookingID string) (Booking, error) {
	bc, err := r.reserveDB.GetCollection(ctx, "reservation-db", "booking")
	if err != nil {
		return Booking{}, err
	}

	var booking Booking
	cur, err := bc.FindOne(ctx, bson.D{{"bookingid", bookingID}})
	if err != nil {
		return Booking{}, err
	}

	ok, err := cur.One(ctx, &booking)
	if err != nil {
		return Booking{}, err
	}

	if !ok {
		return Booking{}, fmt.Errorf("booking not found: %s", bookingID)
	}

	return booking, nil
}

func (r *ReservationServiceImpl) collections(ctx context.Context) (backend.NoSQLCollection, backend.NoSQLCollection, error) {
	rc, err := r.reserveDB.GetCollection(ctx, "reservation-db", "reservation")
	if err != nil {
		return nil, nil, err
	}
	nc, err := r.reserveDB.GetCollection(ctx, "reservation-db", "number")
	if err != nil {
		return nil, nil, err
	}

	return rc, nc, nil
}

func availableForHotel(ctx context.Context, rc backend.NoSQLCollection, nc backend.NoSQLCollection, hotelID, inDate, outDate string, roomCount int64) (bool, error) {
	in, out, err := parseDateRange(inDate, outDate)
	if err != nil {
		return false, err
	}
	capacity, err := hotelCapacity(ctx, nc, hotelID)
	if err != nil {
		return false, err
	}

	start := in
	for start.Before(out) {
		end := start.AddDate(0, 0, 1)
		count, err := reservedRooms(ctx, rc, hotelID, start.Format("2006-01-02"), end.Format("2006-01-02"))
		if err != nil {
			return false, err
		}

		if count+roomCount > capacity {
			return false, nil
		}

		start = end
	}

	return true, nil
}

func reservedRooms(ctx context.Context, c backend.NoSQLCollection, hotelID, inDate, outDate string) (int64, error) {
	var rs []Reservation
	cur, err := c.FindMany(ctx, bson.D{{"hotelid", hotelID}, {"indate", inDate}, {"outdate", outDate}})
	if err != nil {
		return 0, err
	}

	if err = cur.All(ctx, &rs); err != nil {
		return 0, err
	}

	var n int64
	for _, r := range rs {
		n += r.Number
	}

	return n, nil
}

func hotelCapacity(ctx context.Context, c backend.NoSQLCollection, hotelID string) (int64, error) {
	var hn HotelNumber
	cur, err := c.FindOne(ctx, bson.D{{"hotelid", hotelID}})
	if err != nil {
		return 0, err
	}

	ok, err := cur.One(ctx, &hn)
	if err != nil {
		return 0, err
	}

	if !ok {
		return 0, fmt.Errorf("hotel capacity not found: %s", hotelID)
	}

	return hn.Number, nil
}

func insertReservationDays(ctx context.Context, c backend.NoSQLCollection, hotelID, customerName, inDate, outDate string, roomCount int64) error {
	in, out, err := parseDateRange(inDate, outDate)
	if err != nil {
		return err
	}

	start := in
	for start.Before(out) {
		end := start.AddDate(0, 0, 1)
		res := Reservation{
			HotelID:      hotelID,
			CustomerName: customerName,
			InDate:       start.Format("2006-01-02"),
			OutDate:      end.Format("2006-01-02"),
			Number:       roomCount,
		}

		if err = c.InsertOne(ctx, &res); err != nil {
			return err
		}

		start = end
	}

	return nil
}

func parseDateRange(inDate, outDate string) (time.Time, time.Time, error) {
	in, err := time.Parse("2006-01-02", inDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	out, err := time.Parse("2006-01-02", outDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	return in, out, nil
}

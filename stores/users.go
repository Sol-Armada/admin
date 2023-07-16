package stores

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type usersStore struct {
	*mongo.Collection
	ctx context.Context
}

var Users *usersStore

func (u *usersStore) GetContext() context.Context {
	return u.ctx
}

func (u *usersStore) Get(id string) *mongo.SingleResult {
	filter := bson.D{{Key: "_id", Value: id}}
	return u.FindOne(u.ctx, filter)
}

func (u *usersStore) List(filter interface{}) (*mongo.Cursor, error) {
	return u.Find(u.ctx, filter)
}

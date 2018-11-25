package dgraph

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/pkg/errors"
	"github.com/urandom/readeef/content"
	"github.com/urandom/readeef/log"
)

type user struct {
	content.User
	Uid
}

type userInter struct {
	Login       content.Login `json:"login"`
	FirstName   string        `json:"firstName"`
	LastName    string        `json:"lastName"`
	Email       string        `json:"email"`
	HashType    string        `json:"hashType"`
	Admin       bool          `json:"admin"`
	Active      bool          `json:"active"`
	Salt        []byte        `json:"salt"`
	Hash        []byte        `json:"hash"`
	MD5API      []byte        `json:"md5api"`
	ProfileData string        `json:"profileData"`

	Uid
}

func (u user) MarshalJSON() ([]byte, error) {
	res := userInter{
		Login:     u.Login,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Email:     u.Email,
		HashType:  u.HashType,
		Admin:     u.Admin,
		Active:    u.Active,
		Salt:      u.Salt,
		Hash:      u.Hash,
		MD5API:    u.MD5API,
		Uid:       u.Uid,
	}

	b, err := json.Marshal(u.ProfileData)
	if err != nil {
		return nil, errors.WithMessage(err, "marshaling profile data")
	}

	res.ProfileData = string(b)

	return json.Marshal(res)
}

func (u *user) UnmarshalJSON(b []byte) error {
	res := userInter{}
	if err := json.Unmarshal(b, &res); err != nil {
		return errors.WithMessage(err, "unmarshaling intermediate user data")
	}

	var profile content.ProfileData
	if err := json.Unmarshal([]byte(res.ProfileData), &profile); err != nil {
		return errors.WithMessage(err, "unmarshaling profile data")
	}

	u.Login = res.Login
	u.FirstName = res.FirstName
	u.LastName = res.LastName
	u.Email = res.Email
	u.HashType = res.HashType
	u.Admin = res.Admin
	u.Active = res.Active
	u.Salt = res.Salt
	u.Hash = res.Hash
	u.MD5API = res.MD5API
	u.Uid = res.Uid

	u.ProfileData = profile
	return nil
}

type userRepo struct {
	dg *dgo.Dgraph

	log log.Log
}

func (r userRepo) Get(login content.Login) (content.User, error) {
	r.log.Infof("Getting user %s", login)

	resp, err := r.dg.NewReadOnlyTxn().QueryWithVars(context.Background(),
		`query User($login: string) {
user(func: eq(login, $login)) {
	firstName
	lastName
	email
	hashType
	admin
	active
	salt
	hash
	md5api
	profileData
}
}`, map[string]string{
			"$login": string(login),
		})

	if err != nil {
		return content.User{}, errors.Wrapf(err, "getting user %s", login)
	}

	var root struct {
		User []user `json:"user"`
	}

	if err := json.Unmarshal(resp.Json, &root); err != nil {
		return content.User{}, errors.Wrapf(err, "unmarshaling user data for %s", login)
	}

	if len(root.User) == 0 {
		return content.User{}, content.ErrNoContent
	}

	user := root.User[0].User
	user.Login = login

	return user, nil
}

func (r userRepo) All() ([]content.User, error) {
	r.log.Infoln("Getting all users")

	resp, err := r.dg.NewReadOnlyTxn().Query(context.Background(), `{
user(func: has(login)) {
	login
	firstName
	lastName
	email
	hashType
	admin
	active
	salt
	hash
	md5api
	profileData
}
}`)

	if err != nil {
		return nil, errors.Wrap(err, "getting users")
	}

	var root struct {
		User []user `json:"user"`
	}

	if err := json.Unmarshal(resp.Json, &root); err != nil {
		return nil, errors.Wrap(err, "unmarshaling user data")
	}

	users := make([]content.User, 0, len(root.User))

	for _, u := range root.User {
		users = append(users, u.User)
	}

	return users, nil
}

func (r userRepo) Update(u content.User) error {
	if err := u.Validate(); err != nil {
		return errors.WithMessage(err, "validating user")
	}

	ctx := context.Background()
	tx := r.dg.NewTxn()
	defer tx.Discard(ctx)

	resp, err := tx.QueryWithVars(ctx, `
query Uid($login: string) {
	uid(func: eq(login, $login)) {
		uid
	}
}`, map[string]string{"$login": string(u.Login)})
	if err != nil {
		return errors.Wrapf(err, "querying for existing user %s", u)
	}

	var data struct {
		Uid []Uid `json:"uid"`
	}

	if err := json.Unmarshal(resp.Json, &data); err != nil {
		return errors.Wrapf(err, "parsing user query for %s", u)
	}

	var b []byte
	if len(data.Uid) == 0 {
		r.log.Infof("Creating user %s", u)
		b, err = json.Marshal(user{User: u})
	} else {
		r.log.Infof("Updating user %s with uid %d", u, data.Uid[0].ToInt())
		b, err = json.Marshal(user{u, data.Uid[0]})
	}
	if err != nil {
		return errors.Wrapf(err, "marshaling user %s", u)
	}

	_, err = tx.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetJson:   b,
	})

	if err != nil {
		return errors.Wrapf(err, "updating user %s", u)
	}

	return nil
}

func (r userRepo) Delete(u content.User) error {
	if err := u.Validate(); err != nil {
		return errors.WithMessage(err, "validating user")
	}

	r.log.Infof("Deleting user %s", u)

	ctx := context.Background()
	tx := r.dg.NewTxn()
	defer tx.Discard(ctx)

	resp, err := tx.QueryWithVars(ctx, `
query Uid($login: string) {
	uid(func: eq(login, $login)) {
		uid
	}
}`, map[string]string{"$login": string(u.Login)})
	if err != nil {
		return errors.Wrapf(err, "querying for existing user %s", u)
	}

	var data struct {
		Uid []Uid `json:"uid"`
	}

	if err := json.Unmarshal(resp.Json, &data); err != nil {
		return errors.Wrapf(err, "parsing user query for %s", u)
	}

	if len(data.Uid) == 0 {
		return nil
	}

	b, err := json.Marshal(data.Uid[0])
	if err != nil {
		return errors.Wrapf(err, "marshaling uid for user %s", u)
	}

	_, err = tx.Mutate(ctx, &api.Mutation{
		CommitNow:  true,
		DeleteJson: b,
	})

	if err != nil {
		return errors.Wrapf(err, "deleting user %s", u)
	}

	return nil
}

func (r userRepo) FindByMD5(hash []byte) (content.User, error) {
	if len(hash) == 0 {
		return content.User{}, errors.New("no hash")
	}

	r.log.Infof("Getting user using md5 api field %v", hash)

	b64hash := base64.StdEncoding.EncodeToString(hash)
	resp, err := r.dg.NewReadOnlyTxn().QueryWithVars(context.Background(),
		`query User($hash: string) {
user(func: eq(md5api, $hash)) {
	login
	firstName
	lastName
	email
	hashType
	admin
	active
	salt
	hash
	profileData
}
}`, map[string]string{
			"$hash": string(b64hash),
		})

	if err != nil {
		return content.User{}, errors.Wrapf(err, "getting user by md5 %s", hash)
	}

	var root struct {
		User []user `json:"user"`
	}

	if err := json.Unmarshal(resp.Json, &root); err != nil {
		return content.User{}, errors.Wrapf(err, "unmarshaling user data for md5 %s", hash)
	}

	if len(root.User) == 0 {
		return content.User{}, content.ErrNoContent
	}

	user := root.User[0].User
	user.MD5API = hash

	return user, nil
}

package ebs_fields

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/goccy/go-json"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User contains User table in noebs. It should be kept simple and only contain the fields that are needed.
type User struct {
	gorm.Model
	Password        string `binding:"required,min=8,max=20" json:"password"`
	Fullname        string `json:"fullname"`
	Birthday        string `json:"birthday"`
	Mobile          string `json:"mobile" gorm:"primaryKey,index:idx_mobile,unique"`
	Email           string `json:"email"`
	Password2       string `json:"password2" gorm:"-"`
	IsMerchant      bool   `json:"is_merchant" gorm:"default:false"`
	PublicKey       string `json:"user_pubkey"`
	DeviceID        string `json:"device_id"`
	OTP             string `json:"otp"`
	SignedOTP       string `json:"signed_otp"`
	PaymentTokens   []PaymentToken
	Beneficiaries   []Beneficiary
	db              *gorm.DB
	Cards           []Card
	FirebaseIDToken string `json:"firebase_token" gorm:"-"`
	NewPassword     string `json:"new_password" gorm:"-"`
	IsPasswordOTP   bool   `json:"is_password_otp" gorm:"default:false"`
}

func NewUser(db *gorm.DB) *User {
	return &User{
		db: db,
	}
}

// NewUserByMobile Retrieves a user from the database by mobile (username)
func NewUserByMobile(mobile string, db *gorm.DB) (User, error) {
	var user User
	if result := db.First(&user, "mobile = ?", mobile); errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return user, errors.New("user not found")
	}
	user.db = db
	return user, nil
}

func NewUserWithCards(mobile string, db *gorm.DB) (*User, error) {
	var user User
	// Get user model and preload Cards and order the model relation Cards.is_main
	// This trick is really super important: it will allow us to get a user's main card
	// with ease, without having to do a second fetch and then filter the main card
	result := db.Model(&User{}).Preload("Cards", func(db *gorm.DB) *gorm.DB {
		db = db.Order("is_main desc")
		return db
	}).First(&user, "mobile = ?", mobile)
	user.db = db
	return &user, result.Error
}

func NewUserWithBeneficiaries(mobile string, db *gorm.DB) (*User, error) {
	var user User
	// Get user model and preload Beneficiaries and order the model relation
	// This trick is really super important: it will allow us to get a user's main card
	// with ease, without having to do a second fetch and then filter the main card
	result := db.Model(&User{}).Preload("Beneficiaries").First(&user, "mobile = ?", mobile)
	user.db = db
	return &user, result.Error
}

// NewUserByMobile Retrieves a user from the database by mobile (username)
func GetCardsOrFail(mobile string, db *gorm.DB) (*User, error) {
	var user User
	// Get user model and preload Cards and order the model relation Cards.is_main
	// This trick is really super important: it will allow us to get a user's main card
	// with ease, without having to do a second fetch and then filter the main card
	result := db.Model(&User{}).Preload("Cards", func(db *gorm.DB) *gorm.DB {
		db = db.Order("is_main desc")
		return db
	}).First(&user, "mobile = ?", mobile)
	user.db = db
	if len(user.Cards) == 0 {
		return nil, errors.New("empty records")
	}
	return &user, result.Error
}

// NewUserByMobile Retrieves a user from the database by mobile (username)
func GetUserTokens(mobile string, db *gorm.DB) ([]PaymentToken, error) {
	var user User
	// Get user model and preload Cards and order the model relation Cards.is_main
	// This trick is really super important: it will allow us to get a user's main card
	// with ease, without having to do a second fetch and then filter the main card
	result := db.Model(&User{}).Preload("PaymentTokens").First(&user, "mobile = ?", mobile)
	return user.PaymentTokens, result.Error
}

// EncodePublickey a helper function to encode publickey since it has ---BEGIN and new lines
func (u User) EncodePublickey() string {
	return base64.StdEncoding.EncodeToString([]byte(u.PublicKey))
}

func (u *User) SanitizeName() {
	u.Mobile = strings.ToLower(u.Mobile)
}

func (u *User) HashPassword() error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), 8)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	u.Password2 = string(hashedPassword)
	return nil
}

// UpsertCards to an existing noebs user. It uses gorm' relation to amends a user cards
// When adding a card, make sure the card.ID is set to zero value so that
// gorm wouldn't confuse it for an update
//
// FIXME(adonese): since we are using gorm.Model in Card table, gorm thinks
// it is an ID for the card (a primary key) and as a result will do an update instead of insert
func (u User) UpsertCards(cards []Card) error {
	u.Cards = cards
	return u.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Session(&gorm.Session{FullSaveAssociations: true}).Updates(&u).Error
}

//UpsertBeneficiary adds or updates a beneficiary to a user
func (u User) UpsertBeneficiary(beneficiary []Beneficiary) error {
	u.Beneficiaries = beneficiary
	return u.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Session(&gorm.Session{FullSaveAssociations: true}).Updates(&u).Error
}

// UpdateCard only changes a card pan number
func UpdateCard(card Card, db *gorm.DB) error {
	return db.Where("pan = ? AND user_id = ?", card.CardIdx, card.UserID).Updates(&card).Error
}

// DeleteCards soft-deletes a card of list of cards associated to a user
func (u User) DeleteCards(cards []Card) error {
	for idx := range cards {
		cards[idx].UserID = u.ID
	}
	log.Printf("the user card model is: %v", u)
	return u.db.Session(&gorm.Session{FullSaveAssociations: true}).Delete(&cards).Error
}

// DeleteCard with a user_id
func DeleteCard(card Card, db *gorm.DB) error {
	return db.Debug().Where("pan = ? AND user_id = ?", card.CardIdx, card.UserID).Delete(&card).Error
}

// DeleteBeneficiary with a user_id
func DeleteBeneficiary(card Beneficiary, db *gorm.DB) error {
	return db.Debug().Where("data = ? AND user_id = ?", card.Data, card.UserID).Delete(&card).Error
}

type Beneficiary struct {
	Data     string `json:"data"`
	BillType string `json:"bill_type"`
	UserID   uint
}

func NewBeneficiary(number string, billType int, carrier, operator int) Beneficiary {
	var b Beneficiary
	b.Data = number
	switch billType {
	case 0: // it is a telecom
		if operator == 0 { // zain
			if carrier == 0 {
				b.BillType = "0010010001" // prepaid
			} else {
				b.BillType = "0010010002" // postpaid
			}
		} else if operator == 1 { // sudani
			if carrier == 0 {
				b.BillType = "0010010005" // prepaid
			} else {
				b.BillType = "0010010006" // postpaid
			}
		} else { // mtn
			if carrier == 0 {
				b.BillType = "0010010003" // prepaid
			} else {
				b.BillType = "0010010004" // postpaid
			}
		}
	case 1: // nec
		b.BillType = "0010020001"
	case 2: //p2p transfers
		b.BillType = "p2p"
	case 3: // E15
		b.BillType = "0010050001"
	case 4: // bashair
		b.BillType = "0010060002"
	case 5: // mohe Sudan FIXME: we're using the same label for sd and non-sd
		b.BillType = "0010030002"
	case 6: // customs
		b.BillType = "0010030003"
	case 7: // voucher
		b.BillType = "voucher"
	}
	return b
}

// PaymentToken a struct to represent a noebs payment order
// Noebs payment order is an abstraction layer built on top of EBS card transfer
// the idea is to allow noebs users to freely accept and transfer funds, without much of hassle
// that is needed when trying to register as a merchant. Any user can simply generate a payment token
// from noebs companioned apps and then proceed with payment. Another method is to generate a QR code
// which can be scanned by the other end to transfer money.
// A payment token includes the following information, more to come later:
//  1. UUID a unique UUID v4 per each operation, this is requested from ebs via [POST]/payment_token
//  2. ID a unique ID per each payment token, this is an optional field left for the user to supply. In e-commerce cases, an ID represent
//     the order ID.
//  3. Amount the amount to be transferred. Amount is required. A zero amount denotes a free payment.
//  4. UserID the user ID of the user who is making the payment. UserID is required.
//  5. Mobile: the receipient of the payment mobile. This is an optional field
//  6. Note: an optional text note to be sent to the recipient.
type PaymentToken struct {
	gorm.Model `json:",omitempty"`
	UserID     uint     `json:",omitempty"`
	Amount     int      `json:"amount,omitempty"`
	CartID     string   `json:"cart_id,omitempty"`
	UUID       string   `json:"uuid,omitempty"`
	Note       string   `json:"note,omitempty"`
	db         *gorm.DB `gorm:"-"`
	ToCard     string   `json:"toCard,omitempty"` // An optional field to specify the card to be used for payment. Will be updated upon completing the payment.
	// Transaction the transaction associated with the payment token
	Transaction   EBSResponse `json:"transaction,omitempty" gorm:"foreignkey:TransactionID"`
	User          User        `json:",omitempty" binding:"-"` // we shouldn't send back the data to the user
	TransactionID uint        `json:",omitempty"`
	IsPaid        bool        `json:"is_paid"`
}

type qrData struct {
	UUID   string `json:"uuid"`
	ToCard string `json:"toCard,omitempty"`
	Amount int    `json:"amount,omitempty"`
}

// NewPaymentToken creates a new payment token and assign it to a user
func (u *User) NewPaymentToken(amount int, note string, cartID string) (*PaymentToken, error) {
	token := &PaymentToken{
		UserID: u.ID,
		Amount: amount,
		Note:   note,
		CartID: cartID,
	}
	return token, nil
}

// SavePaymentToken saves the payment token to the database
func (u *User) SavePaymentToken(pt *PaymentToken) error {
	return u.db.Model(&PaymentToken{}).Create(pt).Error
}

// GetAllTokens associated to a user.
func GetAllTokens(db *gorm.DB) ([]PaymentToken, error) {
	var tokens []PaymentToken
	result := db.Model(&PaymentToken{}).Preload("Transaction").Find(&tokens)
	return tokens, result.Error
}

// UpsertCards to an existing noebs user. It uses gorm' relation to amends a user cards
// When adding a card, make sure the card.ID is set to zero value so that
// gorm wouldn't confuse it for an update
func (p PaymentToken) UpsertTransaction(transaction EBSResponse, uuid string) error {
	p.Transaction = transaction

	return p.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Session(&gorm.Session{FullSaveAssociations: true}).Where("uuid = ?", uuid).Updates(&p).Error
}

// GetTokenWithTransaction preloads a token with its transaction
func GetTokenWithTransaction(uuid string, db *gorm.DB) (PaymentToken, error) {
	var paymentToken PaymentToken
	result := db.Model(&PaymentToken{}).Preload("Transaction").First(&paymentToken, "uuid = ?", uuid)
	paymentToken.db = db
	return paymentToken, result.Error
}

// GetAllTokens associated to a user.
func GetAllTokensByUserID(userID uint, db *gorm.DB) ([]PaymentToken, error) {
	var tokens []PaymentToken
	result := db.Model(&PaymentToken{}).Preload("Transaction").Where("user_id = ?", userID).Find(&tokens)
	return tokens, result.Error
}

// GetAllTokens associated to a user.
func GetAllTokensByUserIDAndCartID(userID uint, cartID string, db *gorm.DB) ([]PaymentToken, error) {
	// var tokens []PaymentToken
	return nil, nil
}

// NewToken creates a new paymenttoken struct and populate it with a database
func NewToken(db *gorm.DB) *PaymentToken {
	return &PaymentToken{
		db: db,
	}
}

// NewPaymentToken creates a new payment token and assign it to a user
func NewPaymentToken(mobile string, db *gorm.DB) (*PaymentToken, error) {
	user, err := NewUserByMobile(mobile, db)
	if err != nil {
		return nil, err
	}
	token := &PaymentToken{
		UserID: user.ID,
	}
	return token, nil
}

// GetTokenByUUID gets a preloaded token with the user's ID and their cards
func GetTokenWithResult(uuid string, db *gorm.DB) (PaymentToken, error) {
	var payment PaymentToken
	result := db.Debug().Preload("Transaction").First(&payment, "uuid = ?", uuid)
	payment.db = db
	return payment, result.Error
}

// GetTokenByUUID gets a preloaded token with the user's ID and their cards
func GetTokenByUUID(uuid string, db *gorm.DB) (PaymentToken, error) {
	var payment PaymentToken
	result := db.Debug().Preload("User").Preload("User.Cards", func(db *gorm.DB) *gorm.DB {
		db = db.Order("is_main desc, id desc")
		return db
	}).First(&payment, "uuid = ?", uuid)
	payment.db = db
	return payment, result.Error
}

// GetAllTokens associated to a user. This requires a populated model (u.Mobile != "")
func (u *User) GetAllTokens() ([]PaymentToken, error) {
	var user User
	result := u.db.Model(&User{}).Preload("PaymentTokens").Find(&user, "mobile = ?", u.Mobile)
	return user.PaymentTokens, result.Error
}

// Encode PaymentToken to a URL safe link that can be used for online purchases
func Encode(p *PaymentToken) (string, error) {
	var qr qrData
	qr.Amount = p.Amount
	qr.ToCard = p.ToCard
	qr.UUID = p.UUID
	data, err := json.Marshal(qr)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Decode a noebs payment token to an internal PaymentToken that we understand
func Decode(data string) (PaymentToken, error) {
	var p PaymentToken
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(decoded, &p); err != nil {
		return p, err
	}
	return p, nil

}

// Card represents a single card in noebs.
type Card struct {
	gorm.Model
	Pan     string `json:"pan"`
	Expiry  string `json:"exp_date"`
	Name    string `json:"name"`
	IPIN    string `json:"ipin" gorm:"column:ipin"` // set gorm db name to ipin to avoid conflict with the field name in the struct
	UserID  uint
	IsMain  bool   `json:"is_main" gorm:"default:false"`
	CardIdx string `json:"card_index" gorm:"-:all"`
	IsValid *bool  `json:"is_valid"`
}

type CacheCards struct {
	gorm.Model
	Pan     string `json:"pan" gorm:"uniqueIndex"`
	Expiry  string `json:"exp_date"`
	Name    string `json:"name"`
	Mobile  string `json:"mobile" gorm:"-:all"`
	IsValid *bool  `json:"is_valid"`
}

func (c CacheCards) OverrideField() string {
	return "is_valid"
}

func (c CacheCards) GetPk() string {
	return "pan"
}

func (c CacheCards) NewCardFromCached(id int) Card {
	return Card{
		Pan:    c.Pan,
		Expiry: c.Expiry,
		UserID: uint(id),
	}
}

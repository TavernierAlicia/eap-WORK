package main

import (
	"fmt"
	"time"

	eapFact "github.com/TavernierAlicia/eap-FACT"
	eapMail "github.com/TavernierAlicia/eap-MAIL"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
)

func main() {
	// connect database
	//// IMPORT CONFIG ////
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()

	if err != nil {
		fmt.Println("reading config file failed ", err)
	}

	//// DB CONNECTION ////
	pathSQL := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", viper.GetString("database.user"), viper.GetString("database.pass"), viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.dbname"))
	db, err := sqlx.Connect("mysql", pathSQL)

	if err != nil {
		fmt.Println("failed to connect to database ", err)
	}

	// Make fact
	err = fact(db)
	if err != nil {
		fmt.Println("Fact clients failed ", err)
	}

	err = deleteAsked(db)
	if err != nil {
		fmt.Println("Delete accounts failed")
	}
}

// Fact
func fact(db *sqlx.DB) (err error) {
	// select etabs_id who need fact
	var ids []int
	err = db.Select(&ids, "SELECT id FROM etabs WHERE DATE(created_at) = DATE(NOW()) AND suspended != 1 AND offer IS NOT NULL")

	if len(ids) >= 1 {
		for _, id := range ids {
			var etab eapFact.FactEtab

			// get data to create fact
			err = db.Get(&etab, "SELECT name, owner_civility, owner_name, owner_surname, mail, phone, fact_addr, fact_cp, fact_city, fact_country, offer FROM etabs WHERE etabs.id = ?", id)

			if err != nil {
				report(db, id, 0, "GET_FAILED")
				fmt.Println("Get etab data failed ", err)
			} else {
				// get offer
				err = db.Get(&etab.Etab_offer, "SELECT id, name, priceHT, priceTTC FROM offers WHERE id = ?", etab.Offer)
				if err != nil {
					report(db, id, 0, "GET_FAILED")
				} else {
					etab.Fact_infos.Uuid = uuid.New().String()
					etab.Fact_infos.IsFirst = true
					etab.Fact_infos.Date = time.Now().Format("02-01-2006")
					etab.Fact_infos.Link = "./media/factures/" + etab.Fact_infos.Uuid + "_" + etab.Fact_infos.Date + ".pdf"

					// create new fact
					insertFact, err := db.Exec("INSERT INTO factures (uuid, etab_id, link, created) VALUES (?, ?, ?, NOW())", etab.Fact_infos.Uuid, id, etab.Fact_infos.Link)
					if err != nil {
						report(db, id, etab.Fact_infos.Id, "INSERT_FAILED")
					} else {
						factID, err := insertFact.LastInsertId()
						if err != nil {
							report(db, id, etab.Fact_infos.Id, "INSERT_FAILED")
						} else {
							etab.Fact_infos.Id = factID
							err = eapFact.CreateFact(etab)
							if err != nil {
								report(db, id, etab.Fact_infos.Id, "CREATE_FAILED")
							} else {
								err = eapMail.SendBossFact(etab)
								if err != nil {
									report(db, id, etab.Fact_infos.Id, "SEND_FAILED")
								} else {
									report(db, id, etab.Fact_infos.Id, "SENDED")
								}
							}
						}
					}
				}
			}
		}
	}
	return err
}

func deleteAsked(db *sqlx.DB) (err error) {
	var ids []int
	// get accounts to delete
	err = db.Select(&ids, "SELECT id FROM etabs WHERE DATE(created_at) = DATE(NOW()) AND offer IS NULL")

	if len(ids) >= 1 {
		for _, id := range ids {

			// get more infos
			var etab eapFact.FactEtab
			err = db.Get(&etab, "SELECT name, owner_civility, owner_name, owner_surname, mail, phone, fact_addr, fact_cp, fact_city, fact_country, 0 AS offer FROM etabs WHERE etabs.id = ?", id)
			if err != nil {
				fmt.Println("cannot get infos: ", err)
			} else {
				// delete all data for this account
				_, err = db.Exec("DELETE FROM conections WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting connections: ", err)
				}
				_, err = db.Exec("DELETE FROM factures WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting factures: ", err)
				}
				_, err = db.Exec("DELETE FROM fact_logs WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting fact_logs: ", err)
				}
				_, err = db.Exec("DELETE FROM items WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting items: ", err)
				}
				_, err = db.Exec("DELETE orders, order_items FROM orders INNER JOIN order_items WHERE orders.id = order_items.order_id AND etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting orders and order_items: ", err)
				}
				_, err = db.Exec("DELETE FROM planning WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting planning: ", err)
				}
				_, err = db.Exec("DELETE FROM qr_tokens WHERE etab_id = ?", id)
				if err != nil {
					fmt.Println("Error deleting qr_tokens: ", err)
				}
				_, err = db.Exec("DELETE FROM etabs WHERE id = ?", id)
				if err != nil {
					fmt.Println("Error deleting etabs: ", err)
				}
			}
			// now send mail
			err = eapMail.ConfirmDeleteAccount(etab)
			if err != nil {
				fmt.Println("send mail delete failed")
			}
		}
	}
	return err
}

// Report bug
func report(db *sqlx.DB, etab_id int, fact_id int64, status string) {
	_, err := db.Exec("INSERT INTO fact_logs (fact_id, etab_id, status, created_at) VALUES (?, ?, ?, NOW())", fact_id, etab_id, status)

	if err != nil {
		fmt.Println("Error logging send results: ", err)
	}
}

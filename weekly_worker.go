package main

import (
	"fmt"

	eapFact "github.com/TavernierAlicia/eap-FACT"
	eapMail "github.com/TavernierAlicia/eap-MAIL"
	_ "github.com/go-sql-driver/mysql"
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

	err = remindFact(db)
	if err != nil {
		fmt.Println("Cannot remind fact to clients: ", err)
	}

}

// Fact reminder
func remindFact(db *sqlx.DB) (err error) {
	// select etabs who didn't pay
	var ids []int
	err = db.Select(&ids, "SELECT etab_id FROM factures JOIN etabs ON etabs.id = factures.etab_id WHERE payed is NULL AND etabs.suspended != 1 GROUP BY etab_id")
	if len(ids) >= 1 {
		for _, id := range ids {
			// get more details
			var etab eapFact.FactEtab
			err = db.Get(&etab, "SELECT name, owner_civility, owner_name, owner_surname, mail, phone, fact_addr, fact_cp, fact_city, fact_country, offer FROM etabs WHERE etabs.id = ?", id)
			if err != nil {
				fmt.Println("cannot get infos: ", err)
			} else {
				var unpaid eapMail.Unpaid
				err = db.Get(&unpaid, `
					SELECT 
						COUNT(factures.id) AS number,
						(offers.priceTTC * COUNT(factures.id)) AS total
					FROM factures 
						JOIN etabs ON etabs.id = factures.etab_id 
						JOIN offers ON offers.id = etabs.offer
					WHERE factures.payed is NULL AND etab_id = ?`,
					id)

				if err != nil {
					fmt.Println("Cannot get unpayed: ", err)
				} else {
					if unpaid.Number > 2 {
						_, err = db.Exec("UPDATE etabs SET suspended = 1 WHERE id = ?", id)
						if err != nil {
							fmt.Println("accunt suspend failed: ", err)
						} else {
							err = eapMail.SuspendCreanceMail(etab, unpaid)
							if err != nil {
								fmt.Println("send warning failed: ", err)
							}
						}
					} else {
						var result []string
						err = db.Select(&result, "SELECT link FROM factures WHERE payed is NULL AND etab_id = ?", id)
						if err != nil {
							fmt.Println("cannot get fact links: ", err)
						} else {
							unpaid.Facts = result
							// send mail now
							err = eapMail.CreanceMail(etab, unpaid)
							if err != nil {
								fmt.Println("cannot send fact reminder: ", err)
							}
						}
					}
				}
			}
		}
	}

	return err
}

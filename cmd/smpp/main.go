package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/linxGnu/gosmpp"
	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

// type SmppService struct {
// 	trans *gosmpp.Session
// }

// func NewSmppService() *SmppService {
// 	return &SmppService{
// 		trans: sendingAndReceiveSMS(),
// 	}
// }

func main() {
	trans := newSession()
	defer trans.Close()

	go func() {
		http.HandleFunc("/incoming_message", func(w http.ResponseWriter, r *http.Request) {
			msg, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Println("error read incoming message body")
				return
			}
			log.Println("=======>" + string(msg))
			fmt.Fprint(w, "Ok")
		})
		log.Fatal(http.ListenAndServe(":8081", nil))
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := trans.Transceiver().Submit(newSubmitSM("test message")); err != nil {
			fmt.Fprintf(w, "error: %v", err)
		} else {
			fmt.Fprint(w, "Ok")
		}
	})
	log.Fatal(http.ListenAndServe(":8080", nil))

}

func newSession() *gosmpp.Session {

	auth := gosmpp.Auth{
		SMSC:       "localhost:2775",
		SystemID:   "169994",
		Password:   "EDXPJU",
		SystemType: "",
	}

	trans, err := gosmpp.NewSession(
		gosmpp.TRXConnector(gosmpp.NonTLSDialer, auth),
		gosmpp.Settings{
			EnquireLink: 5 * time.Second,

			ReadTimeout: 10 * time.Second,

			OnSubmitError: func(_ pdu.PDU, err error) {
				log.Fatal("SubmitPDU error:", err)
			},

			OnReceivingError: func(err error) {
				fmt.Println("Receiving PDU/Network error:", err)
			},

			OnRebindingError: func(err error) {
				fmt.Println("Rebinding but error:", err)
			},

			OnPDU: handlePDU(),

			OnClosed: func(state gosmpp.State) {
				fmt.Println(state)
			},
		}, 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	return trans
	// defer func() {
	// 	_ = trans.Close()
	// }()

	// // sending SMS(s)
	// for i := 0; i < 1800; i++ {
	// 	if err = trans.Transceiver().Submit(newSubmitSM()); err != nil {
	// 		fmt.Println(err)
	// 	}
	// 	time.Sleep(time.Second)
	// }
}

func handlePDU() func(pdu.PDU, bool) {
	concatenated := map[uint8][]string{}
	return func(p pdu.PDU, _ bool) {
		switch pd := p.(type) {
		case *pdu.SubmitSMResp:
			fmt.Printf("SubmitSMResp:%+v\n", pd)

		case *pdu.GenericNack:
			fmt.Println("GenericNack Received")

		case *pdu.EnquireLinkResp:
			fmt.Println("EnquireLinkResp Received")

		case *pdu.DataSM:
			fmt.Printf("DataSM:%+v\n", pd)

		case *pdu.DeliverSM:

			fmt.Printf("DeliverSM:%+v\n", pd)
			log.Println(pd.Message.GetMessage())
			// region concatenated sms (sample code)
			message, err := pd.Message.GetMessage()
			if err != nil {
				log.Fatal(err)
			}
			totalParts, sequence, reference, found := pd.Message.UDH().GetConcatInfo()
			if found {
				if _, ok := concatenated[reference]; !ok {
					concatenated[reference] = make([]string, totalParts)
				}
				concatenated[reference][sequence-1] = message
			}

			values := map[string]string{"message": message}
			json_data, err := json.Marshal(values)
			if err != nil {
				log.Printf("error marshal %v\r\n", err)
			} else {
				_, err := http.Post("http://localhost:8081/incoming_message", "application/json", bytes.NewBuffer(json_data))
				if err != nil {
					log.Printf("error send post request %v\r\n", err)
				}
			}

			if !found {
				log.Println(message)
			} else if parts, ok := concatenated[reference]; ok && isConcatenatedDone(parts, totalParts) {
				log.Println(strings.Join(parts, ""))
				delete(concatenated, reference)
			}
			// endregion
		}
	}
}

func newSubmitSM(msg string) *pdu.SubmitSM {
	// build up submitSM
	srcAddr := pdu.NewAddress()
	srcAddr.SetTon(5)
	srcAddr.SetNpi(0)
	_ = srcAddr.SetAddress("00" + "522241")

	destAddr := pdu.NewAddress()
	destAddr.SetTon(1)
	destAddr.SetNpi(1)
	_ = destAddr.SetAddress("99" + "522241")

	submitSM := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submitSM.SourceAddr = srcAddr
	submitSM.DestAddr = destAddr
	_ = submitSM.Message.SetMessageWithEncoding(msg, data.UCS2)
	submitSM.ProtocolID = 0
	submitSM.RegisteredDelivery = 1
	submitSM.ReplaceIfPresentFlag = 0
	submitSM.EsmClass = 0

	return submitSM
}

func isConcatenatedDone(parts []string, total byte) bool {
	for _, part := range parts {
		if part != "" {
			total--
		}
	}
	return total == 0
}

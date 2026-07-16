package resolve

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestParseDoHIPv4(t *testing.T) {
	response := new(dns.Msg)
	response.SetReply(&dns.Msg{MsgHdr: dns.MsgHdr{Id: 7}, Question: []dns.Question{{Name: "bksxk.nwafu.edu.cn.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}})
	response.Answer = []dns.RR{
		&dns.CNAME{Hdr: dns.RR_Header{Name: "bksxk.nwafu.edu.cn.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET}, Target: "proxy-edu.nwafu.edu.cn."},
		&dns.A{Hdr: dns.RR_Header{Name: "proxy-edu.nwafu.edu.cn.", Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP("210.27.83.20")},
	}
	payload, err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}

	addresses, err := parseDoHIPv4(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := addresses[0].String(), "210.27.83.20"; got != want {
		t.Fatalf("DoH IPv4 = %q, want %q", got, want)
	}
}

func TestParseDoHIPv4RejectsEmptyAnswer(t *testing.T) {
	response := new(dns.Msg)
	response.SetReply(&dns.Msg{MsgHdr: dns.MsgHdr{Id: 8}, Question: []dns.Question{{Name: "missing.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}})
	payload, err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseDoHIPv4(payload); err == nil {
		t.Fatal("empty DoH answer unexpectedly succeeded")
	}
}

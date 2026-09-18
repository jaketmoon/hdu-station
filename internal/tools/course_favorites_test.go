package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
)

func TestFavoriteMergeAndVerification(t *testing.T) {
	for _, mode := range []string{"success", "already", "read_failed", "lost_response", "verify_failed", "missing_original", "unverified", "fit_required"} {
		t.Run(mode, func(t *testing.T) {
			c := NewCampusClient(campusGrant{true})
			s := NewCampusSession(c, nil)
			s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Classes: []Offering{{ClassID: "new", CourseName: "测试课程"}}}}}
			stored := []string{"old"}
			if mode == "already" {
				stored = append(stored, "new")
			}
			posts, reads := 0, 0
			c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/hduhelp-neo/academic/class/fav" || r.Header.Get("X-Staff-Id") != "" {
					t.Fatal("unexpected route")
				}
				if r.Method == "POST" {
					posts++
					var body struct {
						Classes []string `json:"classes"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					if !reflect.DeepEqual(body.Classes, []string{"old", "new"}) {
						t.Fatal("lost existing favorites")
					}
					stored = body.Classes
					if mode == "missing_original" {
						stored = []string{"new"}
					}
					if mode == "lost_response" {
						return nil, errors.New("lost response")
					}
					return campusResponse(200, `{"code":0}`), nil
				}
				reads++
				if mode == "read_failed" || mode == "verify_failed" && reads > 1 {
					return campusResponse(500, `{}`), nil
				}
				data := []map[string]string{}
				for _, id := range stored {
					data = append(data, map[string]string{"classID": id})
				}
				body, _ := json.Marshal(map[string]any{"code": 0, "data": data})
				return campusResponse(200, string(body)), nil
			})
			in := FavoriteInput{ClassIDs: []string{"new", "new"}}
			if mode == "unverified" {
				in.ClassIDs = []string{"invented"}
			}
			if mode == "fit_required" {
				in.RequireFit = true
			}
			r, err := s.AddFavorites(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			want := "confirmed"
			switch mode {
			case "already":
				want = "already_present"
			case "read_failed", "unverified", "fit_required":
				want = "not_written"
			case "verify_failed", "missing_original":
				want = "unknown"
			}
			if r.Status != want {
				t.Fatalf("%s != %s", r.Status, want)
			}
			if (want == "not_written" || mode == "already") && posts != 0 {
				t.Fatal("unexpected write")
			}
			if posts > 1 {
				t.Fatal("retried write")
			}
			if r.Message == "" {
				t.Fatal("missing receipt")
			}
		})
	}
}

type favoriteDeniedGrant struct{}

func (favoriteDeniedGrant) AccessTokenFor(context.Context, string) (string, error) {
	return "", errors.New("denied")
}
func TestFavoriteDeniedScopeDoesNotSendRequest(t *testing.T) {
	c := NewCampusClient(favoriteDeniedGrant{})
	c.http.Transport = campusTransport(func(*http.Request) (*http.Response, error) { t.Fatal("request despite missing grant"); return nil, nil })
	s := NewCampusSession(c, nil)
	s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Classes: []Offering{{ClassID: "a"}}}}}
	r, _ := s.AddFavorites(context.Background(), FavoriteInput{ClassIDs: []string{"a"}})
	if r.Status != "not_written" {
		t.Fatal(r.Status)
	}
}

func TestFavoriteRequiresCurrentPlanAndStopsAfterUnknown(t *testing.T) {
	c := NewCampusClient(campusGrant{true})
	s := NewCampusSession(c, nil)
	s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Classes: []Offering{{ClassID: "a"}}}}}
	s.lastFit = &FitCoursesResult{Offerings: s.lastOfferings, SuggestedPlan: []string{"a"}}
	posts, gets := 0, 0
	c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
			return campusResponse(200, `{"code":0}`), nil
		}
		gets++
		return campusResponse(200, `{"code":0,"data":[]}`), nil
	})
	in := FavoriteInput{ClassIDs: []string{"a"}, RequireFit: true}
	r, _ := s.AddFavorites(context.Background(), in)
	if r.Status != "unknown" || posts != 1 {
		t.Fatal("current plan not accepted")
	}
	s.AddFavorites(context.Background(), in)
	if posts != 1 || gets != 2 {
		t.Fatal("uncertain write retried")
	}
	next := NewCampusSession(c, nil)
	next.lastOfferings = &OfferingResult{Queries: s.lastOfferings.Queries}
	next.lastFit = s.lastFit
	r, _ = next.AddFavorites(context.Background(), in)
	if r.Status != "not_written" || posts != 1 {
		t.Fatal("stale plan accepted")
	}
}

func TestParallelFavoriteSessionsPreserveUnion(t *testing.T) {
	c := NewCampusClient(campusGrant{true})
	stored := []string{"original"}
	c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			var body struct {
				Classes []string `json:"classes"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid body")
			}
			stored = body.Classes
			return campusResponse(200, `{"code":0}`), nil
		}
		data := []map[string]string{}
		for _, id := range stored {
			data = append(data, map[string]string{"classID": id})
		}
		b, _ := json.Marshal(map[string]any{"code": 0, "data": data})
		return campusResponse(200, string(b)), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := NewCampusSession(c, nil)
			id := fmt.Sprint(i)
			s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Classes: []Offering{{ClassID: id}}}}}
			r, _ := s.AddFavorites(context.Background(), FavoriteInput{ClassIDs: []string{id}})
			if r.Status != "confirmed" {
				t.Error("unconfirmed parallel write")
			}
		}(i)
	}
	wg.Wait()
	if len(stored) != 7 || stored[0] != "original" {
		t.Fatal("parallel sessions lost favorites")
	}
}

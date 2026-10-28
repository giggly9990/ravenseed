package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"ravenseed/internal/models"
)

func TestSortDirectionBeforePagination(t *testing.T) {
	f := newFixture(t, false)
	job := f.start(req())
	f.wait(job.ID)
	before := f.calls.Load()
	families, err := f.store.Families(job.ID)
	if err != nil || len(families) == 0 {
		t.Fatal("missing fixture families", err)
	}
	for i := 1; i <= 3; i++ {
		family := families[0]
		family.ID = fmt.Sprintf("sort-%d", i)
		family.Seeders = i * 100
		family.Coverage = i + 2
		family.Newest = time.Unix(int64(i), 0)
		family.Canonical.Resolution = fmt.Sprintf("%dp", i*1000)
		family.Variants = []models.SizeVariant{{Size: int64(i * 1000)}}
		family.TargetStatus.Candidate = true
		payload, err := json.Marshal(family)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.DB.Exec("INSERT INTO release_families VALUES (?,?,?,?)", job.ID, family.ID, i, string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	get := func(endpoint, query string) Page[models.Family] {
		t.Helper()
		code, body := f.do("GET", "/api/v1/searches/"+job.ID+"/"+endpoint+"?"+query, nil, "")
		var page Page[models.Family]
		if code != 200 || json.Unmarshal(body, &page) != nil {
			t.Fatalf("sort: %d %s", code, body)
		}
		return page
	}
	for _, endpoint := range []string{"families", "candidates"} {
		for _, sort := range []string{"coverage", "quality", "seeders", "size", "newest", "target"} {
			query := "sort=" + sort
			desc := get(endpoint, query+"&order=desc")
			defaults := get(endpoint, query)
			if len(desc.Items) < 3 {
				t.Fatal("missing sort fixtures")
			}
			for i, family := range desc.Items {
				if defaults.Items[i].ID != family.ID {
					t.Fatal("default order changed")
				}
				asc := get(endpoint, fmt.Sprintf("%s&order=asc&limit=1&offset=%d", query, len(desc.Items)-1-i))
				if len(asc.Items) != 1 || asc.Items[0].ID != family.ID || asc.Total != desc.Total {
					t.Fatalf("%s %s: sorting did not precede pagination", endpoint, sort)
				}
			}
		}
	}
	if code, _ := f.do("GET", "/api/v1/searches/"+job.ID+"/families?order=invalid", nil, ""); code != 400 {
		t.Fatal("invalid order accepted")
	}
	if f.calls.Load() != before {
		t.Fatal("sorting triggered an upstream search")
	}
}

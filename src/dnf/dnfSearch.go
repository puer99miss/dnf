package dnf

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"set"
)

type Cond struct {
	Key string
	Val string
}

func searchCondCheck(conds []Cond) error {
	if conds == nil || len(conds) == 0 {
		return errors.New("no conds to search")
	}
	m := make(map[string]bool)
	for _, cond := range conds {
		if _, ok := m[cond.Key]; ok {
			return errors.New("duplicate keys: " + cond.Key)
		}
		m[cond.Key] = true
	}
	return nil
}

func (h *Handler) Search(conds []Cond) (docs []int, err error) {
	if err := searchCondCheck(conds); err != nil {
		return nil, err
	}
	termids := make([]int, 0)
	for i := 0; i < len(conds); i++ {
		if id, ok := h.termMap[conds[i].Key+"%"+conds[i].Val]; ok {
			termids = append(termids, id)
		}
	}
	if len(termids) == 0 {
		return nil, errors.New("All cond are not in inverse list")
	}
	return h.doSearch(termids), nil
}

func (h *Handler) doSearch(terms []int) (docs []int) {
	conjs := h.getConjs(terms)
	if len(conjs) == 0 {
		return nil
	}
	return h.getDocs(conjs)
}

func (h *Handler) getDocs(conjs []int) (docs []int) {
	h.conjRvsLock.RLock()
	defer h.conjRvsLock.RUnlock()

	set := set.NewIntSet()

	var wg sync.WaitGroup

	for _, conj := range conjs {
		ASSERT(conj < len(h.conjRvs))
		doclist := h.conjRvs[conj]
		if doclist == nil {
			continue
		}
		for _, doc := range doclist {
			wg.Add(1)
			go func(h *Handler, docid int, w *sync.WaitGroup) {
				h.docs_.RLock()
				defer h.docs_.RUnlock()
				ok, err := h.docs_.docs[docid].attr.Tr.CoverToday()
				if ok {
					set.Add(docid)
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
				w.Done()
			}(h, doc, &wg)
		}
	}
	wg.Wait()
	return set.ToSlice()
}

func (h *Handler) getConjs(terms []int) (conjs []int) {
	h.conjSzRvsLock.RLock()
	defer h.conjSzRvsLock.RUnlock()

	n := len(terms)
	ASSERT(len(h.conjSzRvs) > 0)
	if n >= len(h.conjSzRvs) {
		n = len(h.conjSzRvs) - 1
	}

	conjSet := set.NewIntSet()

	for i := 0; i <= n; i++ {
		termlist := h.conjSzRvs[i]
		if termlist == nil || len(termlist) == 0 {
			continue
		}

		countSet := set.NewCountSet(i)

		for _, tid := range terms {
			idx := sort.Search(len(termlist), func(i int) bool {
				return termlist[i].termId >= tid
			})
			if idx < len(termlist) && termlist[idx].termId == tid &&
				termlist[idx].cList != nil {

				for _, pair := range termlist[idx].cList {
					countSet.Add(pair.conjId, pair.belong)
				}
			}
		}

		/* 处理∅ */
		if i == 0 {
			for _, pair := range termlist[0].cList {
				ASSERT(pair.belong == true)
				countSet.Add(pair.conjId, pair.belong)
			}
		}

		conjSet.AddSlice(countSet.ToSlice())
	}

	return conjSet.ToSlice()
}

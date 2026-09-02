package handler

import (
	"math"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

// paramID parses a numeric route parameter. why strict parsing: GORM treats
// a non-numeric string condition as inline SQL, so feeding it c.Param("id")
// directly is injectable (see docs/e2e-report.md BUG-001); every id-bearing
// route must go through this or an *ByID helper below.
func paramID(c *gin.Context, name string) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id64 == 0 || id64 > math.MaxUint32 {
		return 0, false
	}
	return uint(id64), true
}

func (s *Server) problemByID(c *gin.Context) (*model.Problem, bool) {
	id, ok := paramID(c, "id")
	if !ok {
		return nil, false
	}
	prob := &model.Problem{}
	if err := s.DB.First(prob, id).Error; err != nil {
		return nil, false
	}
	return prob, true
}

func (s *Server) contestByID(c *gin.Context) (*model.Contest, bool) {
	id, ok := paramID(c, "id")
	if !ok {
		return nil, false
	}
	contest := &model.Contest{}
	if err := s.DB.First(contest, id).Error; err != nil {
		return nil, false
	}
	return contest, true
}

// contestByIDValue loads a contest by a raw numeric id (not the route param)
// — for gates that know the contest id from another record, e.g. a problem's
// contest-exclusive copy.
func (s *Server) contestByIDValue(id uint) (*model.Contest, bool) {
	contest := &model.Contest{}
	if err := s.DB.First(contest, id).Error; err != nil {
		return nil, false
	}
	return contest, true
}

func (s *Server) submissionByID(c *gin.Context) (*model.Submission, bool) {
	id, ok := paramID(c, "id")
	if !ok {
		return nil, false
	}
	sub := &model.Submission{}
	if err := s.DB.First(sub, id).Error; err != nil {
		return nil, false
	}
	return sub, true
}

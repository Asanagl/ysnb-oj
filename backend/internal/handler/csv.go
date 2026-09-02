package handler

import (
	"encoding/csv"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// parseUserCSV reads username,student_no,nickname[,password] rows; missing
// passwords get a random one. Rows shorter than 2 fields are errors.
func parseUserCSV(r io.Reader, max int) []importUserRow {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	rows := []importUserRow{}
	for i := 0; i < max; i++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rows = append(rows, importUserRow{Err: "csv parse: " + err.Error()})
			break
		}
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue // blank line
		}
		rows = append(rows, csvRowToUser(record))
	}
	return rows
}

func csvRowToUser(record []string) importUserRow {
	trim := func(i int) string {
		if i < len(record) {
			return strings.TrimSpace(record[i])
		}
		return ""
	}
	row := importUserRow{
		Username:  trim(0),
		StudentNo: trim(1),
		Nickname:  trim(2),
		Password:  trim(3),
	}
	switch {
	case row.Username == "":
		row.Err = "empty username"
	case row.Password == "":
		row.Password = randomCode(10)
	}
	return row
}

// visibleProblemScope filters problems the current user may see; hidden
// problems are setter/admin-only (members/public need any logged-in account).
func visibleProblemScope(db *gorm.DB, c *gin.Context) *gorm.DB {
	claims := auth.CurrentUser(c)
	q := db.Model(&model.Problem{})
	if claims == nil || !isSetterRole(claims.Role) {
		q = q.Where("visibility <> ?", model.VisibilityHidden)
	}
	return q
}

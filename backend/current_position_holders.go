package main

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CurrentPositionHolder คือผู้ดำรงตำแหน่งบริหาร ณ วันนี้ หนึ่งแถวต่อหนึ่งตำแหน่ง
type CurrentPositionHolder struct {
	PositionTitle   string     `json:"positionTitle"`
	DirectoryUserID uint       `json:"directoryUserId"`
	FullName        string     `json:"fullName"`
	StartsOn        *time.Time `json:"startsOn"`
	EndsOn          *time.Time `json:"endsOn"`
	Source          string     `json:"source"`
}

// positionRank ลำดับชั้นตำแหน่งสำหรับเรียงในหน้า: คณบดี → รองคณบดี → ผู้ช่วยคณบดี → หัวหน้า → อื่น ๆ
// ต้องเช็ครองคณบดีและผู้ช่วยคณบดีก่อน เพราะทั้งสองมีคำว่า "คณบดี" อยู่ในชื่อ
// ผู้รักษาการแทนอยู่ลำดับเดียวกับตำแหน่งที่รักษาการ
func positionRank(title string) int {
	key := positionKey(title)
	for _, acting := range []string{"รักษาราชการแทน", "รักษาการแทน"} {
		key = strings.TrimPrefix(key, acting)
	}
	switch {
	case strings.HasPrefix(key, "รองคณบดี"):
		return 1
	case strings.HasPrefix(key, "ผู้ช่วยคณบดี"):
		return 2
	case key == "คณบดี":
		return 0
	case strings.HasPrefix(key, "หัวหน้า"):
		return 3
	default:
		return 4
	}
}

// sortCurrentHolders เรียงตามลำดับชั้นตำแหน่ง แล้วตามชื่อตำแหน่ง แล้วตามชื่อคน
func sortCurrentHolders(items []CurrentPositionHolder) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := positionRank(items[i].PositionTitle), positionRank(items[j].PositionTitle)
		if left != right {
			return left < right
		}
		if items[i].PositionTitle != items[j].PositionTitle {
			return items[i].PositionTitle < items[j].PositionTitle
		}
		return items[i].FullName < items[j].FullName
	})
}

// currentPositionHolders หาผู้ดำรงแต่ละตำแหน่ง ณ วันที่ today ด้วย pickHolder ตัวเดียวกับที่ผูกแถวโดยตำแหน่งในเอกสาร
// หน้านี้จึงแสดงคนเดียวกับที่เอกสารที่ออกวันนี้จะถูกผูก ตำแหน่งที่ไม่มีผู้ดำรงจะไม่ถูกส่งกลับ
func currentPositionHolders(db *gorm.DB, today time.Time) ([]CurrentPositionHolder, error) {
	holdersByKey, err := loadPositionHolders(db)
	if err != nil {
		return nil, err
	}
	picked := make([]*PositionHolder, 0, len(holdersByKey))
	personIDs := make([]uint, 0, len(holdersByKey))
	for _, holders := range holdersByKey {
		if len(holders) == 0 || committeeRoleOnly(holders[0].PositionTitle) {
			continue
		}
		holder := pickHolder(holders, &today)
		if holder == nil {
			continue
		}
		picked = append(picked, holder)
		personIDs = append(personIDs, holder.DirectoryPersonID)
	}
	names := map[uint]string{}
	if len(personIDs) > 0 {
		var people []DirectoryPerson
		if err := db.Where("id IN ?", personIDs).Find(&people).Error; err != nil {
			return nil, err
		}
		for _, person := range people {
			names[person.ID] = person.FullName
		}
	}
	result := make([]CurrentPositionHolder, 0, len(picked))
	for _, holder := range picked {
		result = append(result, CurrentPositionHolder{
			PositionTitle:   holder.PositionTitle,
			DirectoryUserID: holder.DirectoryPersonID,
			FullName:        names[holder.DirectoryPersonID],
			StartsOn:        holder.StartsOn,
			EndsOn:          holder.EndsOn,
			Source:          holder.Source,
		})
	}
	sortCurrentHolders(result)
	return result, nil
}

func listCurrentPositionHolders(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		today := dateOnly(time.Now().In(time.FixedZone("Asia/Bangkok", 7*60*60)))
		holders, err := currentPositionHolders(db, today)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดรายชื่อผู้ดำรงตำแหน่งได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": holders})
	}
}

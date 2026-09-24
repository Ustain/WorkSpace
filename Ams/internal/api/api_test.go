package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ams/internal/api"
	"ams/internal/auth"
	"ams/internal/config"
	"ams/internal/model"
	"ams/internal/service"
	"ams/internal/store"
)

// env 是一套跑在临时 SQLite 上的完整服务，每个用例独立一份，互不干扰。
type env struct {
	t      *testing.T
	router http.Handler
	db     *gorm.DB
	admin  string // 管理员 token
	user   string // 普通用户 token
}

type resp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// TestMain 关掉 gin 的访问日志，否则一次全量测试会刷出几千行噪音。
func TestMain(m *testing.M) {
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	// 必须显式关闭，否则 Windows 下 TempDir 清理会因文件被占用而失败。
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := store.Seed(db); err != nil {
		t.Fatalf("灌入演示数据失败: %v", err)
	}

	cfg := config.Load()
	cfg.JWTSecret = "test-secret"
	cfg.TokenTTL = time.Hour

	settings := service.NewSettingService(db)
	notifier := service.NewNotifier(db, settings, cfg.SMTP)
	wf := service.NewWorkflow(db, notifier, settings)
	rm := service.NewReminder(db, notifier, settings)
	jwtMgr := auth.NewManager(cfg.JWTSecret, cfg.TokenTTL)

	srv := api.NewServer(db, cfg, jwtMgr, settings, notifier, wf, rm)
	e := &env{t: t, router: srv.Router(fstest.MapFS{}), db: db}
	e.admin = e.login("admin", "admin123")
	e.user = e.login("zhouheng", "user123")
	return e
}

func (e *env) do(method, path, token string, body any) (int, resp) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	var r resp
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return w.Code, r
}

// mustOK 断言请求成功并把 data 解码到 out。
func (e *env) mustOK(method, path, token string, body, out any) {
	e.t.Helper()
	status, r := e.do(method, path, token, body)
	if status != http.StatusOK || r.Code != 0 {
		e.t.Fatalf("%s %s 期望成功，实际 status=%d code=%d msg=%s", method, path, status, r.Code, r.Message)
	}
	if out != nil {
		if err := json.Unmarshal(r.Data, out); err != nil {
			e.t.Fatalf("解析响应失败: %v (原始: %s)", err, r.Data)
		}
	}
}

func (e *env) login(username, password string) string {
	e.t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	e.mustOK(http.MethodPost, "/api/auth/login", "",
		map[string]string{"username": username, "password": password}, &out)
	if out.Token == "" {
		e.t.Fatalf("登录 %s 未拿到 token", username)
	}
	return out.Token
}

type pageResp struct {
	Total int64             `json:"total"`
	List  []json.RawMessage `json:"list"`
}

type assetJSON struct {
	ID         uint       `json:"id"`
	AssetNo    string     `json:"assetNo"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	HolderID   uint       `json:"holderId"`
	DueAt      *time.Time `json:"dueAt"`
	CategoryID uint       `json:"categoryId"`
}

func (e *env) listAssets(query, token string) (int64, []assetJSON) {
	e.t.Helper()
	var p struct {
		Total int64       `json:"total"`
		List  []assetJSON `json:"list"`
	}
	e.mustOK(http.MethodGet, "/api/assets"+query, token, nil, &p)
	return p.Total, p.List
}

func (e *env) getAsset(id uint) assetJSON {
	e.t.Helper()
	var a assetJSON
	e.mustOK(http.MethodGet, fmt.Sprintf("/api/assets/%d", id), e.admin, nil, &a)
	return a
}

// newAsset 现场造一台干净的在库设备。
// 演示数据里前几台在库设备已带有待审批申请，直接拿来跑流程会撞上防重复校验。
func (e *env) newAsset(name string) assetJSON {
	e.t.Helper()
	var created assetJSON
	e.mustOK(http.MethodPost, "/api/assets", e.admin, map[string]any{
		"name": name, "categoryId": 1, "brand": "TestBrand", "model": "T-1",
		"status": model.AssetInStock, "location": "测试仓库", "price": 8888,
	}, &created)
	return created
}

// ---------------------------------------------------------------------------

func TestLoginRejectsBadCredentials(t *testing.T) {
	e := newEnv(t)
	status, r := e.do(http.MethodPost, "/api/auth/login", "",
		map[string]string{"username": "admin", "password": "wrong"})
	if status != http.StatusUnauthorized {
		t.Fatalf("错误密码应返回 401，实际 %d", status)
	}
	// 不应泄露"用户存在但密码错"这类信息。
	if r.Message != "用户名或密码错误" {
		t.Errorf("提示信息不应区分用户名与密码错误，实际: %s", r.Message)
	}
}

func TestRBACBlocksUnauthorizedAccess(t *testing.T) {
	e := newEnv(t)

	cases := []struct {
		name, method, path, token string
		want                      int
	}{
		{"未登录访问设备列表", http.MethodGet, "/api/assets", "", http.StatusUnauthorized},
		{"普通用户访问用户管理", http.MethodGet, "/api/users", e.user, http.StatusForbidden},
		{"普通用户访问审计日志", http.MethodGet, "/api/audit-logs", e.user, http.StatusForbidden},
		{"普通用户新增设备", http.MethodPost, "/api/assets", e.user, http.StatusForbidden},
		{"管理员访问用户管理", http.MethodGet, "/api/users", e.admin, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, _ := e.do(c.method, c.path, c.token, nil)
			if status != c.want {
				t.Errorf("期望 %d，实际 %d", c.want, status)
			}
		})
	}
}

func TestAssetFilters(t *testing.T) {
	e := newEnv(t)

	total, _ := e.listAssets("?pageSize=1", e.admin)
	if total < 100 {
		t.Fatalf("演示数据应有上百台设备，实际 %d", total)
	}

	t.Run("按状态过滤", func(t *testing.T) {
		_, list := e.listAssets("?status=in_use&pageSize=200", e.admin)
		if len(list) == 0 {
			t.Fatal("应存在领用中的设备")
		}
		for _, a := range list {
			if a.Status != model.AssetInUse {
				t.Fatalf("过滤结果混入了 %s 状态的设备", a.Status)
			}
		}
	})

	t.Run("状态多选", func(t *testing.T) {
		onlyStock, _ := e.listAssets("?status=in_stock&pageSize=1", e.admin)
		onlyUse, _ := e.listAssets("?status=in_use&pageSize=1", e.admin)
		both, _ := e.listAssets("?status=in_stock,in_use&pageSize=1", e.admin)
		if both != onlyStock+onlyUse {
			t.Errorf("多选结果 %d 应等于 %d + %d", both, onlyStock, onlyUse)
		}
	})

	t.Run("到期时间范围", func(t *testing.T) {
		// 取一台领用中的设备，用它的到期日当天做区间，必须能查到它自己。
		_, inUse := e.listAssets("?status=in_use&pageSize=200", e.admin)
		var probe *assetJSON
		for i := range inUse {
			if inUse[i].DueAt != nil {
				probe = &inUse[i]
				break
			}
		}
		if probe == nil {
			t.Skip("演示数据中没有带到期日的设备")
		}
		day := probe.DueAt.Format("2006-01-02")
		_, hits := e.listAssets("?dueFrom="+day+"&dueTo="+day+"&pageSize=200", e.admin)
		found := false
		for _, a := range hits {
			if a.ID == probe.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("到期日为 %s 的设备 %s 未被区间查询命中（边界应闭合到当天 23:59:59）", day, probe.AssetNo)
		}
	})

	t.Run("逾期快捷筛选", func(t *testing.T) {
		_, list := e.listAssets("?quick=overdue&pageSize=200", e.admin)
		if len(list) == 0 {
			t.Fatal("演示数据应构造出逾期设备")
		}
		now := time.Now()
		for _, a := range list {
			if a.DueAt == nil || a.DueAt.After(now) {
				t.Fatalf("设备 %s 未逾期却出现在逾期筛选中", a.AssetNo)
			}
		}
	})

	t.Run("关键字搜索", func(t *testing.T) {
		_, list := e.listAssets("?keyword=ThinkPad&pageSize=200", e.admin)
		if len(list) == 0 {
			t.Error("按品牌关键字应能搜到设备")
		}
	})

	t.Run("普通用户只看得到自己的设备", func(t *testing.T) {
		_, list := e.listAssets("?mine=1&status=in_use&pageSize=200", e.user)
		var me struct {
			ID uint `json:"id"`
		}
		e.mustOK(http.MethodGet, "/api/auth/profile", e.user, nil, &me)
		for _, a := range list {
			if a.HolderID != me.ID {
				t.Fatalf("mine=1 返回了他人持有的设备 %s", a.AssetNo)
			}
		}
	})
}

// TestBorrowLifecycle 走一遍领用 → 续借 → 归还的完整闭环，
// 这是整个系统最核心的状态机，任何一步出错都会导致台账与实物对不上。
func TestBorrowLifecycle(t *testing.T) {
	e := newEnv(t)

	target := e.newAsset("生命周期测试笔记本")

	var me struct {
		ID uint `json:"id"`
	}
	e.mustOK(http.MethodGet, "/api/auth/profile", e.user, nil, &me)

	// --- 提交领用申请 ---
	var created struct {
		Application struct {
			ID      uint   `json:"id"`
			ApplyNo string `json:"applyNo"`
			Status  string `json:"status"`
		} `json:"application"`
	}
	e.mustOK(http.MethodPost, "/api/applications", e.user, map[string]any{
		"type": "borrow", "assetId": target.ID, "reason": "单元测试：课题数据处理", "loanDays": 30,
	}, &created)
	if created.Application.Status != model.StatusPending {
		t.Fatalf("新建申请应为待审批，实际 %s", created.Application.Status)
	}

	// 申请阶段不应改动设备状态，否则"申请了但没批"会把设备占死。
	if got := e.getAsset(target.ID); got.Status != model.AssetInStock {
		t.Errorf("提交申请后设备状态应保持在库，实际 %s", got.Status)
	}

	t.Run("重复申请被拦截", func(t *testing.T) {
		status, r := e.do(http.MethodPost, "/api/applications", e.user, map[string]any{
			"type": "borrow", "assetId": target.ID, "reason": "重复提交", "loanDays": 30,
		})
		if status == http.StatusOK {
			t.Errorf("同一设备的重复待审批申请应被拒绝，实际却成功了（%s）", r.Message)
		}
	})

	t.Run("申请人不能自己审批", func(t *testing.T) {
		status, _ := e.do(http.MethodPost,
			fmt.Sprintf("/api/applications/%d/approve", created.Application.ID), e.user,
			map[string]string{"comment": "自批"})
		if status != http.StatusForbidden {
			t.Errorf("普通用户审批应返回 403，实际 %d", status)
		}
	})

	// --- 审批通过 ---
	e.mustOK(http.MethodPost,
		fmt.Sprintf("/api/applications/%d/approve", created.Application.ID), e.admin,
		map[string]string{"comment": "同意领用"}, nil)

	after := e.getAsset(target.ID)
	if after.Status != model.AssetInUse {
		t.Fatalf("审批通过后设备应为领用中，实际 %s", after.Status)
	}
	if after.HolderID != me.ID {
		t.Errorf("领用人应为申请人 %d，实际 %d", me.ID, after.HolderID)
	}
	if after.DueAt == nil {
		t.Fatal("审批通过后应写入到期时间")
	}

	t.Run("申请人收到审批结果通知", func(t *testing.T) {
		var p pageResp
		e.mustOK(http.MethodGet, "/api/notifications?type=approval&pageSize=5", e.user, nil, &p)
		if p.Total == 0 {
			t.Error("审批通过后申请人应收到站内信")
		}
	})

	t.Run("资产履历已记录", func(t *testing.T) {
		var records []struct {
			Action string `json:"action"`
		}
		e.mustOK(http.MethodGet, fmt.Sprintf("/api/assets/%d/records", target.ID), e.admin, nil, &records)
		if len(records) < 2 {
			t.Errorf("领用后履历至少应有入库与领用两条，实际 %d 条", len(records))
		}
	})

	// --- 续借：到期日应在原到期日基础上顺延 ---
	var renew struct {
		Application struct {
			ID uint `json:"id"`
		} `json:"application"`
	}
	e.mustOK(http.MethodPost, "/api/applications", e.user, map[string]any{
		"type": "renew", "assetId": target.ID, "reason": "单元测试：课题未结项", "loanDays": 60,
	}, &renew)
	e.mustOK(http.MethodPost,
		fmt.Sprintf("/api/applications/%d/approve", renew.Application.ID), e.admin,
		map[string]string{"comment": "同意续借"}, nil)

	renewed := e.getAsset(target.ID)
	if renewed.DueAt == nil || !renewed.DueAt.After(*after.DueAt) {
		t.Errorf("续借后到期时间应顺延，原 %v 现 %v", after.DueAt, renewed.DueAt)
	}
	if gap := renewed.DueAt.Sub(*after.DueAt).Hours() / 24; gap < 59 || gap > 61 {
		t.Errorf("续借 60 天应顺延约 60 天，实际 %.1f 天", gap)
	}

	// --- 归还 ---
	var ret struct {
		Application struct {
			ID uint `json:"id"`
		} `json:"application"`
	}
	e.mustOK(http.MethodPost, "/api/applications", e.user, map[string]any{
		"type": "return", "assetId": target.ID, "reason": "单元测试：归还",
	}, &ret)
	e.mustOK(http.MethodPost,
		fmt.Sprintf("/api/applications/%d/approve", ret.Application.ID), e.admin,
		map[string]string{"comment": "已核验"}, nil)

	returned := e.getAsset(target.ID)
	if returned.Status != model.AssetInStock {
		t.Errorf("归还后设备应回到在库，实际 %s", returned.Status)
	}
	if returned.HolderID != 0 || returned.DueAt != nil {
		t.Errorf("归还后应清空领用人与到期时间，实际 holder=%d due=%v", returned.HolderID, returned.DueAt)
	}
}

func TestRejectDoesNotChangeAssetStatus(t *testing.T) {
	e := newEnv(t)
	target := e.newAsset("驳回测试显示器")

	var created struct {
		Application struct {
			ID uint `json:"id"`
		} `json:"application"`
	}
	e.mustOK(http.MethodPost, "/api/applications", e.user, map[string]any{
		"type": "borrow", "assetId": target.ID, "reason": "单元测试", "loanDays": 10,
	}, &created)

	t.Run("驳回必须填理由", func(t *testing.T) {
		status, _ := e.do(http.MethodPost,
			fmt.Sprintf("/api/applications/%d/reject", created.Application.ID), e.admin,
			map[string]string{"comment": "  "})
		if status == http.StatusOK {
			t.Error("空驳回理由应被拒绝")
		}
	})

	e.mustOK(http.MethodPost,
		fmt.Sprintf("/api/applications/%d/reject", created.Application.ID), e.admin,
		map[string]string{"comment": "库存紧张"}, nil)

	if got := e.getAsset(target.ID); got.Status != model.AssetInStock {
		t.Errorf("驳回不应改变设备状态，实际 %s", got.Status)
	}

	t.Run("已处理的申请不能重复审批", func(t *testing.T) {
		status, _ := e.do(http.MethodPost,
			fmt.Sprintf("/api/applications/%d/approve", created.Application.ID), e.admin,
			map[string]string{"comment": "再批一次"})
		if status == http.StatusOK {
			t.Error("重复审批应被拒绝")
		}
	})
}

func TestRepairFlow(t *testing.T) {
	e := newEnv(t)
	_, mine := e.listAssets("?mine=1&status=in_use&pageSize=1", e.user)
	if len(mine) == 0 {
		t.Skip("该演示用户当前没有在用设备")
	}
	target := mine[0]

	var created struct {
		Ticket struct {
			ID uint `json:"id"`
		} `json:"ticket"`
	}
	e.mustOK(http.MethodPost, "/api/repairs", e.user, map[string]any{
		"assetId": target.ID, "fault": "单元测试：风扇异响",
	}, &created)

	if got := e.getAsset(target.ID); got.Status != model.AssetRepairing {
		t.Errorf("报修后设备应转入维修中，实际 %s", got.Status)
	}

	// 修复后不回库房，应退还给原领用人。
	e.mustOK(http.MethodPost, fmt.Sprintf("/api/repairs/%d/handle", created.Ticket.ID), e.admin,
		map[string]any{"status": "resolved", "solution": "更换风扇", "cost": 120, "backToStock": false}, nil)

	fixed := e.getAsset(target.ID)
	if fixed.Status != model.AssetInUse {
		t.Errorf("修复后应退还原领用人（领用中），实际 %s", fixed.Status)
	}
	if fixed.HolderID != target.HolderID {
		t.Errorf("修复后领用人不应变化")
	}
}

func TestBatchOperationSkipsInUseAssets(t *testing.T) {
	e := newEnv(t)
	_, inUse := e.listAssets("?status=in_use&pageSize=2", e.admin)
	if len(inUse) == 0 {
		t.Skip("演示数据不足")
	}
	stock := []assetJSON{e.newAsset("批量测试设备")}

	ids := []uint{inUse[0].ID, stock[0].ID}
	var res struct {
		Affected int      `json:"affected"`
		Skipped  []string `json:"skipped"`
	}
	e.mustOK(http.MethodPost, "/api/assets/batch", e.admin,
		map[string]any{"ids": ids, "action": "scrap", "remark": "单元测试报废"}, &res)

	if res.Affected != 1 {
		t.Errorf("只有在库设备应被报废，实际影响 %d 台", res.Affected)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("领用中的设备应被跳过，实际跳过 %d 台", len(res.Skipped))
	}
	if got := e.getAsset(inUse[0].ID); got.Status != model.AssetInUse {
		t.Errorf("领用中的设备状态不应被批量操作改动，实际 %s", got.Status)
	}
	if got := e.getAsset(stock[0].ID); got.Status != model.AssetScrapped {
		t.Errorf("在库设备应被报废，实际 %s", got.Status)
	}
}

func TestImportReportsPerRowErrors(t *testing.T) {
	e := newEnv(t)

	header := "资产编号,设备名称,设备类型,品牌,型号,序列号,规格配置,状态,存放位置,采购价格,采购日期,保修到期,领用人,领用部门,领用时间,到期时间,备注\n"
	rows := header +
		",导入测试笔记本,笔记本电脑,TestBrand,T-100,SNTEST001,i5/16G,在库,A座5层办公区,6999,2025-06-01,2028-06-01,,,,,导入测试\n" +
		",类型不存在的行,不存在的类型,X,Y,SNTEST002,,在库,,100,,,,,,,应当失败\n" +
		",缺领用人的行,笔记本电脑,X,Y,SNTEST003,,领用中,,100,,,,,,,应当失败\n" +
		",价格非法的行,笔记本电脑,X,Y,SNTEST004,,在库,,abc,,,,,,,应当失败\n"

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "import.csv")
	fw.Write([]byte(rows))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/assets/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+e.admin)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	var r resp
	json.Unmarshal(w.Body.Bytes(), &r)
	if w.Code != http.StatusOK || r.Code != 0 {
		t.Fatalf("导入接口失败: status=%d msg=%s", w.Code, r.Message)
	}

	var out service.ImportResult
	json.Unmarshal(r.Data, &out)
	if out.Created != 1 {
		t.Errorf("应成功导入 1 条，实际 %d", out.Created)
	}
	if out.Failed != 3 {
		t.Errorf("应有 3 行校验失败，实际 %d", out.Failed)
	}
	if len(out.Errors) != 3 {
		t.Fatalf("错误报告应包含 3 条明细，实际 %d", len(out.Errors))
	}
	// 行号要对得上，用户才能回到 Excel 里精确定位。
	for i, want := range []int{3, 4, 5} {
		if out.Errors[i].Row != want {
			t.Errorf("第 %d 条错误的行号应为 %d，实际 %d", i+1, want, out.Errors[i].Row)
		}
	}
}

func TestExportProducesDownloadableFiles(t *testing.T) {
	e := newEnv(t)

	for _, tc := range []struct {
		format      string
		minSize     int
		contentType string
	}{
		{"xlsx", 10000, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"csv", 5000, "text/csv"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/assets/export?format="+tc.format, nil)
			req.Header.Set("Authorization", "Bearer "+e.admin)
			w := httptest.NewRecorder()
			e.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("导出失败 status=%d", w.Code)
			}
			if got := w.Body.Len(); got < tc.minSize {
				t.Errorf("导出内容过小：%d 字节", got)
			}
			if ct := w.Header().Get("Content-Type"); ct == "" {
				t.Error("缺少 Content-Type")
			}
			if cd := w.Header().Get("Content-Disposition"); cd == "" {
				t.Error("缺少 Content-Disposition，浏览器不会触发下载")
			}
		})
	}

	t.Run("导出遵循当前筛选条件", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/assets/export?format=csv&status=in_stock", nil)
		req.Header.Set("Authorization", "Bearer "+e.admin)
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, req)

		all, _ := e.listAssets("?status=in_stock&pageSize=1", e.admin)
		// CSV 行数 = 表头 + 记录数。
		lines := bytes.Count(w.Body.Bytes(), []byte("\n"))
		if int64(lines) != all+1 {
			t.Errorf("导出行数 %d 与筛选结果 %d+1 不符", lines, all)
		}
	})
}

func TestStatsEndpoints(t *testing.T) {
	e := newEnv(t)

	var ov struct {
		TotalAssets  int64   `json:"totalAssets"`
		InUse        int64   `json:"inUse"`
		InStock      int64   `json:"inStock"`
		Overdue      int64   `json:"overdue"`
		TotalValue   float64 `json:"totalValue"`
		UsageRate    float64 `json:"usageRate"`
		PendingApply int64   `json:"pendingApply"`
	}
	e.mustOK(http.MethodGet, "/api/stats/overview", e.admin, nil, &ov)
	if ov.TotalAssets == 0 || ov.TotalValue == 0 {
		t.Fatal("概览统计不应为空")
	}
	if ov.UsageRate < 0 || ov.UsageRate > 100 {
		t.Errorf("在用率应在 0~100 之间，实际 %.2f", ov.UsageRate)
	}

	var charts struct {
		ByCategory   []struct{ Name string } `json:"byCategory"`
		ByStatus     []struct{ Name string } `json:"byStatus"`
		MonthlyTrend struct {
			Labels []string `json:"labels"`
			Borrow []int    `json:"borrow"`
		} `json:"monthlyTrend"`
		DueForecast struct {
			Labels []string `json:"labels"`
			Counts []int    `json:"counts"`
		} `json:"dueForecast"`
	}
	e.mustOK(http.MethodGet, "/api/stats/charts", e.admin, nil, &charts)
	if len(charts.ByCategory) == 0 || len(charts.ByStatus) == 0 {
		t.Error("类型/状态分布不应为空")
	}
	if len(charts.MonthlyTrend.Labels) != 12 || len(charts.MonthlyTrend.Borrow) != 12 {
		t.Errorf("月度趋势应为 12 个月，实际 %d", len(charts.MonthlyTrend.Labels))
	}
	// 9 = 1 个"已逾期" + 未来 8 周。
	if len(charts.DueForecast.Labels) != 9 || len(charts.DueForecast.Counts) != 9 {
		t.Errorf("到期预测应为 9 个分桶，实际 %d", len(charts.DueForecast.Labels))
	}

	var mine struct {
		MyAssets int64 `json:"myAssets"`
		Unread   int64 `json:"unread"`
	}
	e.mustOK(http.MethodGet, "/api/stats/my", e.user, nil, &mine)
}

func TestAuditLogRecordsWritesAndMasksPasswords(t *testing.T) {
	e := newEnv(t)

	// 创建一个用户，其密码字段必须在审计日志里被脱敏。
	e.mustOK(http.MethodPost, "/api/users", e.admin, map[string]any{
		"username": "audituser", "password": "secret123", "realName": "审计测试",
		"roleId": 3, "departmentId": 1,
	}, nil)

	var p struct {
		Total int64 `json:"total"`
		List  []struct {
			Username string `json:"username"`
			Module   string `json:"module"`
			Detail   string `json:"detail"`
		} `json:"list"`
	}
	e.mustOK(http.MethodGet, "/api/audit-logs?module=users&pageSize=5", e.admin, nil, &p)
	if p.Total == 0 {
		t.Fatal("新增用户的操作应被审计记录")
	}
	entry := p.List[0]
	if entry.Username != "admin" {
		t.Errorf("审计记录的操作人应为 admin，实际 %s", entry.Username)
	}
	if bytes.Contains([]byte(entry.Detail), []byte("secret123")) {
		t.Errorf("审计日志泄露了明文密码: %s", entry.Detail)
	}
	if !bytes.Contains([]byte(entry.Detail), []byte("***")) {
		t.Errorf("密码字段应被替换为 ***，实际: %s", entry.Detail)
	}
}

func TestDeleteGuards(t *testing.T) {
	e := newEnv(t)

	t.Run("领用中的设备不能删除", func(t *testing.T) {
		_, inUse := e.listAssets("?status=in_use&pageSize=1", e.admin)
		status, _ := e.do(http.MethodDelete, fmt.Sprintf("/api/assets/%d", inUse[0].ID), e.admin, nil)
		if status == http.StatusOK {
			t.Error("领用中的设备应禁止删除")
		}
	})

	t.Run("内置角色不能删除", func(t *testing.T) {
		status, _ := e.do(http.MethodDelete, "/api/roles/1", e.admin, nil)
		if status == http.StatusOK {
			t.Error("系统内置角色应禁止删除")
		}
	})

	t.Run("管理员不能删除自己", func(t *testing.T) {
		var me struct {
			ID uint `json:"id"`
		}
		e.mustOK(http.MethodGet, "/api/auth/profile", e.admin, nil, &me)
		status, _ := e.do(http.MethodDelete, fmt.Sprintf("/api/users/%d", me.ID), e.admin, nil)
		if status == http.StatusOK {
			t.Error("不应允许删除当前登录账号")
		}
	})
}

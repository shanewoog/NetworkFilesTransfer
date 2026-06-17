package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Config maps the top-level fields in config.json.
type Config struct {
	UploadMode        string `json:"upload_mode"`
	Port              string `json:"port"`
	Domain            string `json:"domain"`
	UploadDir         string `json:"upload_dir"`
	TempDir           string `json:"temp_dir"`
	DBPath            string `json:"db_path"`
	ExpireHours       int    `json:"expire_hours"`
	MaxSingleSizeGB   int64  `json:"max_single_size_gb"`
	MaxTotalSizeGB    int64  `json:"max_total_size_gb"`
	ShareCodeLength   int    `json:"share_code_length"`
	DownloadLimit     int    `json:"download_limit"`
	RetentionInterval int    `json:"retention_interval_min"`
}

var (
	db         *sql.DB
	globalCfg  Config
	appVersion string

	tempCleanupMu     sync.Mutex
	lastTempCleanupAt time.Time
)

const (
	defaultShareCodeLength = 4
	defaultDownloadLimit   = 5
	defaultVersionPrefix   = "1.0"
	maxFileNameLen         = 180
	tempUploadStaleAfter   = 6 * time.Hour
	tempCleanupMinInterval = 5 * time.Minute
	cancelMarkerMaxAge     = 24 * time.Hour
)

func main() {
	// 1. 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囨繝寰锋澘鈧洟宕姘辨殾闁哄被鍎查悡鏇犫偓鍏夊亾闁逞屽墴瀹曟洟骞嬮悩鐢殿槸闂佸搫绋侀崢浠嬫偂濞嗘挻鐓熸俊銈傚亾闁绘锕﹀▎銏ゆ嚑椤掑倻锛滈梺閫炲苯澧柣锝嗙箞瀹曠喖顢楅崒姘闂佽楠哥粻宥夊磿鏉堫煈娈介柛娑橈功椤╁弶绻濇繝鍌涘櫧缁炬儳銈搁弻锝呂熼搹瑙勭€繝銏ｎ潐钃遍柕鍥у椤㈡﹢鍨鹃崘鎻捫ユ俊鐐€ч梽鍕偂閿熺姴钃熸繛鎴欏灪閸嬪嫰鏌涘▎蹇ｆЧ闁绘繃鐗滅槐鎾寸瑹閸パ勭彯闂佸憡锚閵堟悂骞冮幆褉鏀介悗锝庝簽椤︽澘顪冮妶鍡楃瑨闁稿﹤鎽滈弫顔芥償閵婏妇鍘介柟鑹版彧缁辨洟鎮鹃銏＄厱閹肩补鈧疇鍩為柣鎾卞€濋弻鏇熺箾閻愵剚鐝旈梺?
	loadConfig()
	loadVersion()
	initEnv()
	initDB()
	initR2Replication()
	initR2DirectUploader()
	defer db.Close()

	// 2. 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫鎾绘偐閼碱剦妲遍柣鐔哥矌婢ф鏁幒妤€鍨傞柛宀€鍋為悡鐔镐繆閵堝倸浜鹃梺缁橆殔閿曨亜鐣烽敐鍫㈢杸婵炴垶鐟ч崢鎾绘⒑閸涘﹦绠撻悗姘煎弮瀹曞疇銇愰幒鎾充画濠电偛妫欓悷褔鎮鹃崹顐闁绘劕顕晶閬嶆煕閹烘挸娴€规洘顨嗗鍕節娴ｅ壊妫滈梻鍌欐祰椤宕曢悡骞盯宕熼杞版睏濠电偛妫欓幐濠氭偂閻旂厧绠规繛锝庡墮閻掓椽鏌涢悢椋庣闁哄本鐩幃鈺佺暦閸パ€鎷伴柣搴ゎ潐濞叉﹢宕濆鈧俊鐢稿箛閺夎法顔婂┑鐘绘涧濞诧箑鈻嶅畝鍕拻濞达綀娅ｇ敮娑㈡煕閺冣偓濞茬喖骞嗛崶銊ヮ嚤闁哄鍨归崝锕€顪冮妶鍡楃瑐缂佲偓娴ｈ褰掝敊闁款垰浜鹃悷娆忓绾炬悂鏌涢弮鈧崹鍧楀Υ娴ｇ硶妲堟俊顖炴敱椤秴鈹戦埥鍡楃仩闁搞垹寮剁粋宥嗐偅閸愨晝鍘介梺褰掑亰閸撴岸骞嗛崼銏㈢＜闁绘瑦鐟ュú锕傛偂閸愵喖绾ч柣鎰綑椤ュ鏌涢弬璺ㄐ㈡い顓℃硶閹瑰嫰宕崟鍏哥棯缂傚倷鑳剁划顖滄崲閸儱鏄ラ柍褜鍓氶妵鍕箳閹存繍浠鹃梺鍝勬噺閹倿寮婚敐鍛傜喖宕崟顓㈢崜婵＄偑鍊戦崕鑼崲閸繍娼栨繛宸簻缁犲綊鏌ｉ幇顓炵祷濠殿喚鍏樺濠氬磼濮橆剦浠奸柣搴㈢煯閸楁娊濡存笟鈧鎾閳ュ磭妾┑鐘灱椤鎹㈤崒婊呯濠电姴娲ょ粻鏍煃閸濆嫬鈧鎮风憴鍕╀簻闁哄秲鍔庨。鍙夈亜閺傚灝顏╅柍?
	go startCleanupTask()

	r := gin.Default()

	// 3. 闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌涢…鎴濅簽妞も晜褰冮湁闁绘ê妯婇崕蹇曠磼閳ь剚寰勭仦绋夸壕闁稿繐顦禍楣冩⒑閸涘﹥澶勯柛鎾寸懄缁傚秴螖閳ь剟鍩為幋锔藉€烽柛娆忣樈濡繝姊洪崨濠冣拹妞ゃ劌锕畷娲閵堝懐鐫勯梺閫炲苯澧村┑锛勬暬瀹曠喖顢涘槌栧晪闂備礁鎲￠〃鍫ュ磻閻斿摜顩峰Δ锝呭暞閳锋垿鏌熺粙鎸庢崳闁靛棙甯￠弻娑㈡偐閺屻儱寮板┑鐘亾濞撴埃鍋撴慨濠呮缁辨帒顫滈崱娆忓Ъ闂佽绻愮换鎴︽偡閳轰胶鏆︽繛宸簼閺呮彃顭跨捄渚剳闁告ê宕埞鎴﹀煡閸℃浠銈庡幖閻楁捇宕洪埀顒併亜閹哄秶璐版繛鍫熒戞穱濠囧矗婢跺﹤顫掗悗娈垮櫘閸ｏ絽鐣锋總鍓叉晝闁挎繂妫欓悵銊╂⒒閸屾瑨鍏岀痪顓炵埣瀹曟粌鈹戠€ｎ偄浠悷婊勬濡喖姊洪崘鍙夋儓闁瑰啿绻橀崺娑㈠箣閻樼數锛滈柣搴秵閸樼晫娑甸崜浣虹＜闁绘瑥鎳愮粙鑽ょ磼缂佹娲寸€殿喖鐖奸獮瀣倷閸欏浜峰┑锛勫亼閸婃垿宕瑰ú顏呭仭闁靛ě鍛瑝濠电偞鍨惰彜婵℃彃鐗婃穱濠囶敍濠х偓瀚涢梺鍛婅壘椤戝顫忕紒妯肩懝闁逞屽墴閸┾偓妞ゆ帒鍊告禒婊堟煠濞茶鐏￠柡鍛閳ь剛鏁哥涵鍫曞磻閹炬枼鏋旈柛顭戝枟閻忓牓姊虹拠鑼闁煎綊绠栭、姘跺Ψ閳轰胶顦板銈嗗笂缁€渚€宕濋崨瀛樷拺闂傚牊绋掔粋瀣箾閻撳孩鍋ラ柛鈹惧亾濡炪倖宸婚崑鎾绘煥閺囥劋绨绘い顐㈢箻閹煎綊鎮烽弶娆惧殭闂備礁鎼ú銊╁窗閹扮増鍋熼柡宥庡幗閳锋垿姊婚崼姘珖缂佸娉涢埞鎴﹀灳閾忣偅鎮欓柛?
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Upload-ID, X-Index")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Increase the in-memory multipart buffer limit.
	r.MaxMultipartMemory = 256 << 20

	// 4. 闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繒鈧娲栧ú锔藉垔娴煎瓨鐓ラ柡鍥╁仜閳ь剚鎮傚畷褰掑磼閻愬鍘甸梺璇″灣婢ф藟婢舵劖鐓涢悗锝傛櫇缁愭梹鎱ㄦ繝鍐┿仢妞ゃ垺顨婇崺鈧い鎺戝€婚惌鎾绘煙缂併垹鏋涢柣顓燁殔閳规垿鎮╅崣澶婎槱闂佺粯鎸鹃崰鎰┍婵犲浂鏁嶆慨妯哄綁閾忓酣姊哄Ч鍥у闁稿簺鍊楅幑?
	r.StaticFile("/", "./index.html") // 闂傚倸鍊搁崐鎼佸磹閻戣姤鍤勯柛顐ｆ礀缁犵娀鏌熼崜褏甯涢柛濠呭煐閹便劌螣閾忛€涢偗闂侀€炲苯澧剧紒鐘虫尭閻ｇ兘宕奸弴銊︽櫌婵犮垼娉涢鍡椻枍瀹ュ鈷掑ù锝呮憸缁夌儤淇婇銉︾《缂侇喖鐗婇幏鍛存惞閸︻厾鍔堕梻浣稿閸嬧偓闁瑰啿娲畷鎴﹀箻閼姐倕绁﹂梺鍓茬厛閸犳牗鎱ㄦ惔鈾€鏀介柣鎰摠缂嶆垿鏌涙繝鍌涜础闁逞屽墴濞佳囧Χ缁嬭法鏆﹂柟鐑樺焾濞尖晠鏌ㄥ┑鍡橈紞婵炲吋妫冨缁樻媴閻戞ê娈屾繝鈷€鍌滅煓妞ゃ垺顨婇獮鎾诲箳濠靛洨绋佹繝寰锋澘鈧洟宕幍顔碱棜濠靛倻顭堢痪褔鏌涢顐簻濞存粍绮撻弻锝夊箻閺夋垵顫掗梺鍝勬湰濞茬喎鐣烽幆閭︽Щ濡炪倕娴氶崣鍐蓟閻旂⒈鏁婇柤濮愬€楅悡鎾绘倵鐟欏嫭纾搁柛鏂跨Ф閹广垹鈹戠€ｎ亞顦ㄩ梺鎸庢⒒閹虫挻绂?
	r.StaticFile("/app.css", "./app.css")
	r.StaticFile("/qrcode.min.js", "./qrcode.min.js")
	r.StaticFile("/favicon.svg", "./favicon.svg")
	r.StaticFile("/download", "./download.html")
	r.GET("/:code", func(c *gin.Context) {
		c.File("./download.html")
	})

	api := r.Group("/api")
	{
		api.GET("/config", getConfig)
		api.GET("/storage", getStorageInfo)
		api.GET("/file/:code", getFileInfo)            // 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾鐎规洏鍎抽埀顒婄秵閸犳牜澹曢崸妤佺厵闁诡垳澧楅ˉ澶愬箹閺夋埊韬柡灞诲€濋幊婵嬪箥椤旇偐澧┑鐐茬摠缁瞼绱炴繝鍥ц摕婵炴垯鍨瑰敮濡炪倖姊婚崢褔锝為埡浣叉斀闁宠棄妫楁禍婵嬫煥閺囨ê鐏茬€殿喛顕ч埥澶愬閻樻牓鍔嶉妵鍕冀椤愵澀娌銈呴缁夋挳鈥旈崘顔嘉ч柛鈩冾殔椤垵鈹戦埥鍡椾簻閻庢凹鍨甸妵鎰瑹閳ь剙顫忓ú顏勭闁绘劖褰冩慨椋庣磽娴ｈ棄钄煎┑顔哄€濆鏌ュ醇閺囩偟顔岄梺?
		api.POST("/upload/check", checkFileExist)      // 缂傚倸鍊搁崐鎼佸磹閹间礁纾瑰瀣捣閻棗銆掑锝呬壕闁芥ɑ绻堥弻鐔封枔閸喗鐏嶉梺鍝勬缁矂鍩為幋锔藉亹闁圭粯甯╂禒鎯ь渻閵堝骸浜濇繛鑼枛瀵鈽夐姀鈺傛櫇闂佹寧绻傚Λ娑⑺囬妷鈺傗拺闁告稑锕ゆ慨鈧梺绋款儐閹瑰洤顫忕紒妯诲闁兼亽鍎抽妴濠囨⒑閸濄儱校闁圭顭烽獮鍫ュΩ閵壯勬疂闂佽顔栭崰妤€鈻?
		api.POST("/upload/cancel", handleCancelUpload) // 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫鎾绘偐閸愯弓鐢婚梻浣瑰濞叉牠宕愯ぐ鎺戠；閻庯綆鍋傜换鍡涙煏閸繃鍣归柡鍡欏枛閺岋綁顢橀悢绋跨３濠殿喖锕ュ钘夌暦瑜版帩鏁嬮柛娑卞幖婢瑰姊绘担鍝ワ紞缂侇噮鍨跺濠氬Ω閳轰胶鍘撮梺纭呮彧闂勫嫰宕戦幇鏉跨骇闁割偒鍋勬禍婊堟煕濮楀牏绡€婵?
		api.POST("/upload/chunk", handleChunk)         // 闂傚倸鍊搁崐鎼佸磹瀹勬噴褰掑炊椤掑﹦绋忔繝銏ｆ硾椤戝洭銆呴幓鎹楀綊鎮╁顔煎壈缂佺偓鍎冲锟犲蓟閿涘嫪娌悹鍥ㄥ絻婵酣姊洪崫鍕靛剮闁煎啿鐖奸獮澶愬箹娴ｇ懓浜遍梺鍓插亝缁诲嫰鎮烽妸褏纾藉ù锝嗗絻娴滈箖姊虹化鏇炲⒉缂佸甯￠幃锟犲即閵忥紕鍘撻梺瀹犳〃缁€渚€寮抽悢鍏肩厵闁告劖褰冮弳鐐烘煏?
		api.POST("/upload/merge", handleMerge)         // 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫鎾绘偐閸愬弶鐤勫┑掳鍊х徊浠嬪疮椤栫偛纾婚悗锝庡枟閻撴洟鏌嶉埡浣告殶闁愁垱娲熼弻娑㈠Χ閸℃顫掗梺鍝勭焿缂嶄礁顕ｉ鈧畷鎺戭潩椤戣法甯涚紓鍌氬€风拋鏌ュ磻閹剧粯鐓曢柍鈺佸暟閳洟鏌ｉ幘瀛樼闁哄苯绉归崺鈩冩媴閸涘﹥顔嶉梻浣烘嚀閸熷潡宕鐐参?
		api.POST("/r2/upload/init", handleR2UploadInit)
		api.POST("/r2/upload/sign-part", handleR2UploadSignPart)
		api.POST("/r2/upload/complete", handleR2UploadComplete)
		api.POST("/r2/upload/cancel", handleR2UploadCancel)
		api.GET("/download/temp/:token", tempDownload)
		api.GET("/download/:code", download)
	}

	// Short download URL.
	r.GET("/temp/:token", tempDownload)

	fmt.Printf("NetworkFilesTransfer service started on port %s\n", globalCfg.Port)
	if err := r.Run(":" + globalCfg.Port); err != nil {
		log.Fatalf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫鎾绘偐閼碱剦妲遍柣鐔哥矌婢ф鏁幒妤€鍨傞柛宀€鍋為悡鐔镐繆閵堝倸浜鹃梺缁橆殔閿曨亜鐣烽敐鍫㈢杸婵炴垶鐟ч崢鎾绘⒑閸涘﹦绠撻悗姘煎弮瀹曞疇銇愰幒鎾斥偓鍨叏濮楀棗澧绘俊鍙夋そ閺屽秷顧侀柛鎾寸懅缁顓兼径濠勶紵闂備緡鍓欑粔瀵哥不閻樿绠归柟纰卞幘閸樻粎绱撳鍡欏⒌闁哄本娲熷畷鐓庘攽閸パ屸偓娑㈡⒑缂佹ɑ鐓ユ俊顐ｇ箞瀵鎮㈢喊杈ㄦ櫖濠电偞鍨堕敃鈺佄涢崱娆戠＝濞达綁娼ф俊鍏肩箾绾绡€妤? %v", err)
	}
}

// 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾鐎规洏鍎抽埀顒婄秵閸犳牜澹曢崸妤佺厵闁诡垳澧楅ˉ澶愬箹閺夋埊韬柡灞诲€濋幊婵嬪箥椤旇偐澧┑鐐茬摠缁瞼绱炴繝鍥ц摕婵炴垯鍨瑰敮濡炪倖姊婚崢褔锝為鍫熲拺缂備焦蓱鐏忋劑鏌涚€ｎ偅宕岄柡宀嬬稻閹棃鏁嶉崟顓熸闂備胶顭堟绋跨暦椤掑倸寮查梻渚€娼ц墝闁哄懏鐩畷娆撴偐缂佹鍘遍悗鍏夊亾闁逞屽墴瀹曟垿鎮欓悜妯轰簵濠电偛妫欓幐濠氭偂濞嗘劑浜滈柟鎹愭硾閺嬪海绱掑Δ浣告诞闁哄本鐩顕€鍩€椤掆偓椤繈濡搁埡浣虹枀?
func getConfig(c *gin.Context) {
	c.JSON(200, gin.H{
		"domain":       globalCfg.Domain,
		"expire_hours": globalCfg.ExpireHours,
		"upload_mode":  effectiveUploadMode(),
		"version":      appVersion,
	})
}

// 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾鐎规洏鍎抽埀顒婄秵閸犳牜澹曢崸妤佺厵闁诡垳澧楅ˉ澶愬箹閺夋埊韬柡灞诲€濋幊婵嬪箥椤旇偐澧┑鐐茬摠缁瞼绱炴繝鍥ц摕婵炴垯鍨瑰敮闂侀潧绻嗛崜婵嗩熆閹烘鈷戠紒瀣儥閸庢劙鏌熼崨濠冨€愰柛鈹垮劜瀵板嫭绻涢悙顒傗偓鍝勵渻閵堝棙瀵欓柛宀€鍋涙禒鎰版煟鎼淬値娼愭繛鍙夛耿瀹曞綊鎮介崜鍙夋櫍婵犻潧鍊婚…鍫ユ倷婵犲洦鐓冮柛婵嗗閺嗙喖鏌涘Ο鐓庮暭缂佺粯绻堥幃浠嬫濞戞鎹曟繝纰樻閸ㄤ即鏁冮妷褏鐭夌€广儱鎳夐弨浠嬫倵閿濆骸浜濇繛鍛墵濮婅櫣绱掑Ο娲绘濠电偟鈷堟禍婵嬪箚閺冨牆惟闁靛／鍐ｅ亾閻愮儤鈷戦柟绋挎捣缁犳捇鏌熼崘鏌ュ弰闁糕晜绋掑鍕偓锝庡墰椤?
func getStorageInfo(c *gin.Context) {
	uSize, err := currentManagedStorageBytes("")
	if err != nil {
		log.Printf("query managed storage size failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query storage info failed"})
		return
	}
	maxSize := globalCfg.MaxTotalSizeGB * 1024 * 1024 * 1024
	available := maxSize - uSize
	if available < 0 {
		available = 0
	}
	c.JSON(200, gin.H{
		"used_gb":      float64(uSize) / 1024 / 1024 / 1024,
		"available_gb": float64(available) / 1024 / 1024 / 1024,
	})
}

// 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸崹楣冨箛娴ｅ湱绋佺紓鍌氬€烽悞锕佹懌闂佸憡鐟ョ换姗€寮婚悢纰辨晬闁挎繂娲ｅЧ?config.json 闂傚倸鍊搁崐鎼佸磹閹间礁纾归柣鎴ｅГ閸婂潡鏌ㄩ弴妤€浜惧銈庡幖閻忔繆鐏掗梺鍏肩ゴ閺呮繈鎮＄€ｎ喗鈷戦柛鎾村絻娴滄繃绻涢崣澶涜€跨€规洘鍨块、鏃堝醇閻斿搫骞堥梻浣规灱閺呮盯宕导鏉戠叀濠㈣泛顑勭换鍡樸亜閹板墎绉堕柤鏉挎健閺岋紕浠﹂崜褎鍒涙繝纰夌磿閸忔﹢銆佸☉銏″€烽柟纰卞幐閸嬫挸顓奸崪浣瑰瘜?
// Load config.json.
func loadConfig() {
	content, err := os.ReadFile("config.json")
	if err != nil {
		globalCfg = Config{
			UploadMode:        "",
			Port:              "9000",
			Domain:            "http://localhost:9000",
			UploadDir:         "./uploads",
			TempDir:           "./temp",
			DBPath:            "./share.db",
			ExpireHours:       24,
			MaxSingleSizeGB:   10,
			MaxTotalSizeGB:    20,
			ShareCodeLength:   defaultShareCodeLength,
			DownloadLimit:     defaultDownloadLimit,
			RetentionInterval: 10,
		}
		globalR2Cfg = R2Config{}
		return
	}
	if err := json.Unmarshal(content, &globalCfg); err != nil {
		log.Fatalf("read config.json failed: %v", err)
	}
	var cfgWithR2 struct {
		R2 R2Config `json:"r2"`
	}
	if err := json.Unmarshal(content, &cfgWithR2); err != nil {
		log.Fatalf("read R2 config failed: %v", err)
	}
	globalR2Cfg = cfgWithR2.R2.normalized()
	if globalCfg.ShareCodeLength <= 0 {
		globalCfg.ShareCodeLength = defaultShareCodeLength
	}
	if globalCfg.DownloadLimit <= 0 {
		globalCfg.DownloadLimit = defaultDownloadLimit
	}
	globalCfg.UploadMode = normalizedConfiguredUploadMode()
}
func loadVersion() {
	if strings.TrimSpace(appVersion) != "" {
		return
	}

	currentVersion := readStoredVersion()
	appVersion = currentVersion

	exePath, err := os.Executable()
	if err != nil || !shouldAutoBumpVersion(exePath) {
		return
	}

	shouldBump, err := shouldBumpVersionForExecutable(exePath)
	if err != nil {
		log.Printf("婵犵數濮烽弫鍛婃叏閻戝鈧倿鎸婃竟鈺嬬秮瀹曘劑寮堕幋鐙呯幢闂備線鈧偛鑻晶鎾煛鐏炲墽銆掗柍褜鍓ㄧ紞鍡涘磻閸涱厾鏆︾€光偓閸曨剛鍘搁悗鍏夊亾閻庯綆鍓涢敍鐔哥箾鐎电顎撳┑鈥虫喘楠炲繘鎮╃拠鑼唽闂佸湱鍎ら崵鈺呭箣濠靛啯鏂€闂佺粯锕╅崑鍛垔娴煎瓨鐓曢柡鍥╁仧娴犳盯鏌ｉ妶鍕獢闁诡喗顨呴埢鎾诲垂椤旂晫浜鹃梻浣芥〃缁€渚€鈥﹂悜钘壩ュù锝堝€介弮鍫濆窛妞ゆ挾濮存慨锔戒繆閻愵亜鈧牜鏁幒妤€鐤柕濠忓椤╄尙鎲搁悧鍫濈瑲闁绘挻娲滈幉鍛婃償閵婏絺鍋撻崘顔煎窛闁哄鍨归悡鎴炵節閻㈤潧孝闁哥喆鍔嶇粋宥咁煥閸喓鍘搁悗骞垮劚閸燁偅淇婇幐搴涗簻闁挎繂妫涢崣鈧梺鍝勬湰缁嬫帡骞嗛弮鍫濈叀闁告稒婢橀幆鍫熺節濞堝灝鏋涢柨鏇樺€濋垾锕€鐣￠幍顔芥闂佸壊鍋呭ú姗€寮查弻銉︾厱婵炴垵宕晶顔姐亜閵夛箑鍝烘慨濠勭帛閹峰懘鎸婃径濠冨劒闂備礁鎽滄慨鐢稿礉閺囩姴寮查梻浣烘嚀椤曨厽鎱ㄩ悽鍓叉晝闁兼亽鍎查崣蹇斾繆椤栨稑顕滅痪顓㈢畺閺岋綁骞掗幋鐘辩驳闂侀潧娲ょ€氫即鐛幒鎴悑闁搞儴鍩栬ⅵ缂傚倸鍊烽悞锕傘€冮崱娆愭殰婵°倕鎳庢闂佸憡娲﹂崹鏉挎纯闂備礁鎲℃笟妤呭储閽樺鏋旈柡灞诲劜閸婄敻鏌涢…鎴濅簼缂佽埖鐓￠幃妤€顫濋悡搴＄濡炪値鍘煎ú鈺冪不濞戞ǚ妲堥弶鍫涘妽濞呭洭姊? %v", err)
		return
	}
	if !shouldBump {
		return
	}

	nextVersion := nextVersion(currentVersion)
	if err := os.WriteFile("version.txt", []byte(nextVersion), 0666); err != nil {
		log.Printf("write version file failed: %v", err)
		return
	}
	appVersion = nextVersion
}

// 缂傚倸鍊搁崐鎼佸磹閹间礁纾瑰瀣捣閻棗銆掑锝呬壕闁芥ɑ绻堥弻鐔封枔閸喗鐏嶉梺鍝勬缁矂鍩為幋锔藉亹闁圭粯甯╂禒鎯ь渻閵堝骸浜濇繛鑼枛瀵鈽夐姀鈺傛櫇闂佹寧绻傚Λ娑⑺囬妷鈺傗拺闁告稑锕ゆ慨鈧梺绋款儐閹瑰洤顫忕紒妯诲闁兼亽鍎抽妴濠囨⒑閸濄儱校闁圭顭烽獮鍫ュΩ閵壯勬疂闂佽顔栭崰妤€鈻撴ィ鍐┾拺闁圭娴风粻鎾淬亜閿斿灝宓嗛柛鈺傜洴楠炲鎮欓鍐泿闂備浇顫夊妯绘櫠鎼达絿鐭欑紓浣骨滄禍婊勩亜閹板墎绉垫繛鍫燁焽缁?
func checkFileExist(c *gin.Context) {
	var req struct {
		Hash string `json:"hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	req.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
	if !isValidFileHash(req.Hash) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file hash"})
		return
	}

	var code, filePath, primaryBackend string
	var expireAt int64
	var downloadCount, downloadLimit int
	err := db.QueryRow(
		"SELECT code, path, expire_at, COALESCE(download_count, 0), COALESCE(NULLIF(download_limit, 0), ?), COALESCE(NULLIF(primary_backend, ''), ?) FROM files WHERE hash = ?",
		globalCfg.DownloadLimit,
		uploadModeLocal,
		req.Hash,
	).Scan(&code, &filePath, &expireAt, &downloadCount, &downloadLimit, &primaryBackend)
	if err == nil {
		if downloadLimit <= 0 {
			downloadLimit = defaultDownloadLimit
		}
		if time.Now().Unix() >= expireAt || downloadCount >= downloadLimit {
			deleteFileAndRecordByCode(code, filePath)
			c.JSON(http.StatusOK, gin.H{"hit": false})
			return
		}
		if primaryBackend == replicaBackendR2 {
			hasReplica, replicaErr := hasUploadedReplica(code, replicaBackendR2)
			if replicaErr != nil {
				log.Printf("check existing R2 replica failed: code=%s, err=%v", code, replicaErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "check file record failed"})
				return
			}
			if !hasReplica {
				deleteFileRecordByCode(code)
				c.JSON(http.StatusOK, gin.H{"hit": false})
				return
			}
		} else if _, statErr := os.Stat(filePath); statErr != nil {
			if !os.IsNotExist(statErr) {
				log.Printf("check existing instant-upload file failed: %v", statErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "check file record failed"})
				return
			}
			hasReplica, replicaErr := hasUploadedReplica(code, replicaBackendR2)
			if replicaErr != nil {
				log.Printf("check existing R2 replica failed: code=%s, err=%v", code, replicaErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "check file record failed"})
				return
			}
			if !hasReplica {
				deleteFileRecordByCode(code)
				c.JSON(http.StatusOK, gin.H{"hit": false})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"hit": true, "url": "/api/download/" + code, "expire_at": expireAt})
		return
	}
	if err != sql.ErrNoRows {
		log.Printf("query instant-upload record failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query file record failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hit": false})
}
func handleCancelUpload(c *gin.Context) {
	if !isLocalUploadMode() {
		c.JSON(http.StatusNotFound, gin.H{"error": "local upload mode is disabled"})
		return
	}

	var req struct {
		UploadID string `json:"upload_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	req.UploadID = strings.TrimSpace(req.UploadID)
	if !isValidUploadID(req.UploadID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload_id"})
		return
	}

	if err := markUploadCanceled(req.UploadID); err != nil {
		log.Printf("write upload cancel marker failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel upload failed"})
		return
	}
	if err := removeUploadChunks(req.UploadID); err != nil {
		log.Printf("cleanup canceled upload chunks failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel upload failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func handleChunk(c *gin.Context) {
	if !isLocalUploadMode() {
		c.JSON(http.StatusNotFound, gin.H{"error": "local upload mode is disabled"})
		return
	}

	start := time.Now()
	uploadID := strings.TrimSpace(c.GetHeader("X-Upload-ID"))
	indexText := strings.TrimSpace(c.GetHeader("X-Index"))

	if !isValidUploadID(uploadID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload_id"})
		return
	}
	chunkIndex, err := strconv.Atoi(indexText)
	if err != nil || chunkIndex < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk index"})
		return
	}

	if chunkIndex == 0 {
		maybeCleanupStaleTempUploads(uploadID)
	}
	if isUploadCanceled(uploadID) {
		_ = removeUploadChunks(uploadID)
		c.JSON(http.StatusConflict, gin.H{"error": "upload canceled"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("read request body failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "read request body failed"})
		return
	}
	fileSize := int64(len(body))

	uSize, err := currentManagedStorageBytes("")
	if err != nil {
		log.Printf("query managed storage size for chunk upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage check failed"})
		return
	}
	if (uSize + fileSize) > globalCfg.MaxTotalSizeGB*1024*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage limit reached"})
		return
	}

	chunkDir := filepath.Join(globalCfg.TempDir, uploadID)
	if err := os.MkdirAll(chunkDir, os.ModePerm); err != nil {
		log.Printf("create chunk directory failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create chunk directory failed"})
		return
	}
	if isUploadCanceled(uploadID) {
		_ = removeUploadChunks(uploadID)
		c.JSON(http.StatusConflict, gin.H{"error": "upload canceled"})
		return
	}

	chunkPath := filepath.Join(chunkDir, strconv.Itoa(chunkIndex))
	if err := os.WriteFile(chunkPath, body, 0666); err != nil {
		log.Printf("write chunk file failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save chunk failed"})
		return
	}
	if err := touchUploadActive(uploadID); err != nil {
		log.Printf("touch upload activity failed: %v", err)
	}
	if isUploadCanceled(uploadID) {
		_ = removeUploadChunks(uploadID)
		c.JSON(http.StatusConflict, gin.H{"error": "upload canceled"})
		return
	}

	log.Printf("[%dms] chunk upload finished (size=%d)", time.Since(start).Milliseconds(), fileSize)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func handleMerge(c *gin.Context) {
	if !isLocalUploadMode() {
		c.JSON(http.StatusNotFound, gin.H{"error": "local upload mode is disabled"})
		return
	}

	ensureFilesDownloadColumns()

	var req struct {
		UploadID string `json:"upload_id"`
		FileName string `json:"file_name"`
		Total    int    `json:"total"`
		Hash     string `json:"hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	req.UploadID = strings.TrimSpace(req.UploadID)
	req.FileName = sanitizeFileName(req.FileName)
	req.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
	if !isValidUploadID(req.UploadID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload_id"})
		return
	}
	if req.FileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty file name"})
		return
	}
	if req.Total < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid total chunk count"})
		return
	}
	if !isValidFileHash(req.Hash) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file hash"})
		return
	}

	chunkDir := filepath.Join(globalCfg.TempDir, req.UploadID)
	cSize, err := getDirSize(chunkDir)
	if err != nil {
		log.Printf("stat chunk directory failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "chunk directory unavailable"})
		return
	}
	if cSize > globalCfg.MaxSingleSizeGB*1024*1024*1024 {
		if err := os.RemoveAll(chunkDir); err != nil {
			log.Printf("cleanup oversized chunks failed: %v", err)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "file exceeds size limit"})
		return
	}

	finalPath := filepath.Join(globalCfg.UploadDir, req.Hash)
	out, err := os.OpenFile(finalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		if os.IsExist(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "same hash file already exists; retry instant upload check"})
			return
		}
		log.Printf("create final file failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create final file failed"})
		return
	}

	for i := 0; i < req.Total; i++ {
		in, err := os.Open(filepath.Join(chunkDir, fmt.Sprintf("%d", i)))
		if err != nil {
			closeAndRemove(out, finalPath)
			log.Printf("open chunk failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "incomplete chunks"})
			return
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		if copyErr != nil || closeErr != nil {
			closeAndRemove(out, finalPath)
			if copyErr != nil {
				log.Printf("merge chunk failed: %v", copyErr)
			} else {
				log.Printf("close chunk file failed: %v", closeErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "merge file failed"})
			return
		}
	}

	if err := out.Close(); err != nil {
		if removeErr := os.Remove(finalPath); removeErr != nil {
			log.Printf("remove incomplete final file failed: %v", removeErr)
		}
		log.Printf("close final file failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save file failed"})
		return
	}

	fileInfo, err := os.Stat(finalPath)
	if err != nil {
		log.Printf("stat final file failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save file failed"})
		return
	}
	fileSize := fileInfo.Size()
	if ok, err := canReserveManagedStorage(fileSize, ""); err != nil {
		log.Printf("check managed storage for merge failed: %v", err)
		_ = os.Remove(finalPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage check failed"})
		return
	} else if !ok {
		_ = os.Remove(finalPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage limit reached"})
		return
	}

	if err := os.RemoveAll(chunkDir); err != nil {
		log.Printf("cleanup temp chunk dir failed: %v", err)
	}
	if err := os.Remove(cancelMarkerPath(req.UploadID)); err != nil && !os.IsNotExist(err) {
		log.Printf("cleanup upload cancel marker failed: %v", err)
	}

	code := randomString(globalCfg.ShareCodeLength)
	expireAt := time.Now().Add(time.Duration(globalCfg.ExpireHours) * time.Hour).Unix()
	log.Printf("prepare file record: code=%s, name=%s, path=%s, size=%d, expireAt=%d (%s), hash=%s", code, req.FileName, finalPath, fileSize, expireAt, time.Unix(expireAt, 0).Format("2006-01-02 15:04:05"), req.Hash)
	if _, err := db.Exec("INSERT INTO files (code, name, path, size, expire_at, hash, download_count, download_limit, primary_backend) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", code, req.FileName, finalPath, fileSize, expireAt, req.Hash, 0, globalCfg.DownloadLimit, uploadModeLocal); err != nil {
		log.Printf("insert file record failed: %v", err)
		if removeErr := os.Remove(finalPath); removeErr != nil {
			log.Printf("remove file after record insert failure failed: %v", removeErr)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save file record failed"})
		return
	}

	if err := scheduleR2ReplicaUpload(code, finalPath, req.FileName); err != nil {
		log.Printf("schedule R2 upload failed: code=%s, path=%s, err=%v", code, finalPath, err)
	}

	c.JSON(http.StatusOK, gin.H{"code": code, "url": "/api/download/" + code, "expire_at": expireAt})
}
func getFileInfo(c *gin.Context) {
	code := c.Param("code")
	var path, name, primaryBackend string
	var size int64
	var expireAt int64
	var downloadCount, downloadLimit int
	if err := db.QueryRow("SELECT path, name, size, expire_at, COALESCE(download_count, 0), COALESCE(NULLIF(download_limit, 0), ?), COALESCE(NULLIF(primary_backend, ''), ?) FROM files WHERE code = ?", globalCfg.DownloadLimit, uploadModeLocal, code).Scan(&path, &name, &size, &expireAt, &downloadCount, &downloadLimit, &primaryBackend); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}
	if downloadLimit <= 0 {
		downloadLimit = defaultDownloadLimit
	}
	if time.Now().Unix() >= expireAt {
		deleteFileAndRecordByCode(code, path)
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}
	if downloadCount >= downloadLimit {
		deleteFileAndRecordByCode(code, path)
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}
	if primaryBackend == replicaBackendR2 {
		hasReplica, replicaErr := hasUploadedReplica(code, replicaBackendR2)
		if replicaErr != nil {
			log.Printf("check uploaded R2 replica for file info failed: code=%s, err=%v", code, replicaErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query file info failed"})
			return
		}
		if !hasReplica {
			deleteFileAndRecordByCode(code, path)
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
			return
		}
	} else if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("stat local file for file info failed: code=%s, path=%s, err=%v", code, path, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query file info failed"})
			return
		}
		hasReplica, replicaErr := hasUploadedReplica(code, replicaBackendR2)
		if replicaErr != nil {
			log.Printf("check uploaded R2 replica for file info failed: code=%s, err=%v", code, replicaErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query file info failed"})
			return
		}
		if !hasReplica {
			deleteFileAndRecordByCode(code, path)
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"name": name, "size": size, "expire_at": expireAt, "download_count": downloadCount, "download_limit": downloadLimit})
}
func download(c *gin.Context) {
	code := c.Param("code")
	var path, name, primaryBackend string
	var downloadCount, downloadLimit int
	if err := db.QueryRow("SELECT path, name, COALESCE(download_count, 0), COALESCE(NULLIF(download_limit, 0), ?), COALESCE(NULLIF(primary_backend, ''), ?) FROM files WHERE code = ?", globalCfg.DownloadLimit, uploadModeLocal, code).Scan(&path, &name, &downloadCount, &downloadLimit, &primaryBackend); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	if downloadLimit <= 0 {
		downloadLimit = defaultDownloadLimit
	}
	if downloadCount >= downloadLimit {
		deleteFileAndRecordByCode(code, path)
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	if globalR2Cfg.hasAccessDomain() {
		objectKey, err := getUploadedReplicaObjectKey(code, replicaBackendR2)
		if err == nil {
			r2URL := globalR2Cfg.buildAccessURL(objectKey)
			if r2URL != "" {
				if _, err := db.Exec("UPDATE files SET download_count = download_count + 1 WHERE code = ?", code); err != nil {
					log.Printf("increment download count for R2 download failed: code=%s, err=%v", code, err)
				}
				c.JSON(http.StatusOK, gin.H{"url": r2URL, "via": "r2"})
				return
			}
		} else if err != sql.ErrNoRows {
			log.Printf("query uploaded R2 replica failed: code=%s, err=%v", code, err)
		}
	}
	if primaryBackend == replicaBackendR2 {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			deleteFileRecordByCode(code)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	token := randomString(32)
	db.Exec("INSERT INTO temp_tokens (code, token, created_at) VALUES (?, ?, ?)", code, token, time.Now().Unix())
	c.JSON(http.StatusOK, gin.H{"token": token, "url": "/temp/" + token})
}
func tempDownload(c *gin.Context) {
	token := c.Param("token")
	hasRange := c.GetHeader("Range") != ""

	var code string
	var firstDownloadAt int64
	if err := db.QueryRow("SELECT code, first_download_at FROM temp_tokens WHERE token = ?", token).Scan(&code, &firstDownloadAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	var path, name string
	var size int64
	var downloadCount, downloadLimit int
	if err := db.QueryRow("SELECT path, name, COALESCE(size, 0), COALESCE(download_count, 0), COALESCE(NULLIF(download_limit, 0), ?) FROM files WHERE code = ?", globalCfg.DownloadLimit, code).Scan(&path, &name, &size, &downloadCount, &downloadLimit); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
		return
	}

	if !hasRange && firstDownloadAt == 0 {
		if downloadCount >= downloadLimit {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found or expired"})
			return
		}
		db.Exec("UPDATE temp_tokens SET first_download_at = ? WHERE token = ?", time.Now().Unix(), token)
		db.Exec("UPDATE files SET download_count = download_count + 1 WHERE code = ?", code)
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", size))
	c.Header("Content-Disposition", "attachment; filename="+url.PathEscape(name))
	c.Header("Accept-Ranges", "bytes")
	c.File(path)
}
func initEnv() {
	if err := os.MkdirAll(globalCfg.UploadDir, os.ModePerm); err != nil {
		log.Fatalf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囨繝寰锋澘鈧洟宕导瀛樺剹婵炲棙鎸婚悡娆撴倵閻㈡鐒鹃崯鍝ョ磼閹冪稏缂侇喗鐟╁濠氭偄閻撳海鐣鹃悷婊勭矒瀹曠敻鎮㈤崗鑲╁帗缂傚倷鐒﹁摫闁绘挶鍎甸弻宥囨喆閸曨偆浼岄梺璇″枓閺呮粌顭囪箛娑辨晝闁靛繒濮撮懙鎰版⒒閸屾瑨鍏岀紒顕呭灦瀹曟繈寮撮姀鈩冩珖濡炪倕绻愰悧鍡涙嫅閻斿吋鐓ユ繝闈涙－濡叉悂鏌ｉ幘瀵糕槈闁宠鍨块幃鈺呭箵閹哄棗浜剧憸鐗堝笚閸婂爼鏌涢鐘茬伄缁惧彞绮欓弻娑氫沪閻愵剛娈ら悗娑欑箘缁辨挻鎷呴崜鍙夆枅濠电偘鍖犻崶鑸垫櫔闂侀潧顦弲娑氱矆鐎ｎ偁浜滈柟鍝勭Х閸忓瞼绱掓径搴㈢【妞ゎ亜鍟存俊鍫曞幢濡も偓椤洭姊? %v", err)
	}
	if err := os.MkdirAll(globalCfg.TempDir, os.ModePerm); err != nil {
		log.Fatalf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囨繝寰锋澘鈧洟宕导瀛樺剹婵炲棙鎸婚悡娆撴倵閻㈡鐒鹃崯鍝ョ磼閹冪稏缂侇喗鐟╁濠氭偄閻撳海鐣鹃悷婊勭矒瀹曠敻鎮㈤崗鑲╁帗缂傚倷鐒﹁摫妞ゃ儱鐗撻弻鐔碱敊缁涘鐣堕梺瀹犳椤︻垶鍩㈠澶嬫優妞ゆ劑鍨绘导宀勬煢濡崵绠橀柟鍙夋倐瀵噣宕煎┑濠冩啺闂備焦瀵х换鍌炈囬鐐村亗婵炴垶菤閺€浠嬫煟濡绲婚柍褜鍓欑紞濠囧箖濮椻偓瀹曞ジ濮€閵忣澁绱抽梻浣侯焾閺堫剙顫濋妸銉ф懃缂傚倸鍊风粈渚€藝闂堟侗鐒界憸鏃堛€佸璺何ㄩ柍杞拌兌椤︽澘顪冮妶鍡楃瑐闁煎啿澧庣划鏃傛崉娓氼垱瀵岄梺闈涚墕濡鎱ㄩ崒鐐寸厱? %v", err)
	}
	if err := os.MkdirAll(cancelMarkerDir(), os.ModePerm); err != nil {
		log.Fatalf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囨繝寰锋澘鈧洟宕导瀛樺剹婵炲棙鎸婚悡娆撴倵閻㈡鐒鹃崯鍝ョ磼閹冪稏缂侇喗鐟╁濠氭偄閻撳海鐣鹃悷婊勭矒瀹曠敻鎮㈤崗鑲╁帗缂傚倷鐒﹁摫闁绘挶鍎甸弻宥囨喆閸曨偆浼岄梺璇″枓閺呮粌顭囪箛娑辨晝闁靛繒濮撮懙鎰版⒒閸屾瑨鍏岀紒顕呭灦瀹曟繈寮撮姀鈩冩珖濡炪倕绻愰悧鍡涙嫅閻斿摜绠鹃柟瀵稿€戝璺哄嚑閹兼番鍔嶉悡娆撴⒑椤撱劎鐣遍柡瀣⊕閵囧嫰顢曟惔鈩冨櫧缁炬崘妫勯湁闁挎繂鎳庨ˉ蹇斾繆閸欏銇濋柡灞剧缁犳盯寮崱鈺€閭い銏″哺閺佹劙宕卞▎妯荤カ闂佽鍑界紞鍡樼閻愬搫纾归柣鎰▕濞撳鏌曢崼婵囧櫧缂佺姵澹嗙槐鎺撳緞鐎ｎ偄鍞夐悗娈垮櫘閸嬪濡甸幇鏉跨闁圭虎鍨辩€氬ジ姊绘担铏瑰笡闁搞劎鍠栧鎻掝煥閸愶絾鐎洪梺鐟板⒔缁垶鎮￠弴銏＄厽婵☆垵娅ｉ敍宥咁熆瑜戞禍顒傛閹烘挻缍囬柍杞版缁泛顪冮妶鍡樺碍闁靛牏顭堥悾鐑藉础閻愬秵妫冨畷姗€顢旈崱妤冦偊闂傚倸鍊风粈渚€骞栭位鍥敍閻愭潙浜卞┑鐘诧工閻楀棛澹曠拠宸唵閻犲搫鎼惁銊╂煛? %v", err)
	}
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", globalCfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		log.Printf("set SQLite busy_timeout failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		log.Printf("set SQLite WAL mode failed: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE,
		name TEXT,
		path TEXT,
		size INTEGER,
		expire_at INTEGER,
		hash TEXT,
		download_count INTEGER DEFAULT 0,
		download_limit INTEGER,
		primary_backend TEXT DEFAULT 'local'
	);`); err != nil {
		log.Fatalf("init files table failed: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS temp_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT,
		token TEXT UNIQUE,
		created_at INTEGER,
		first_download_at INTEGER DEFAULT 0
	);`); err != nil {
		log.Fatalf("init temp_tokens table failed: %v", err)
	}

	_, err = db.Exec("ALTER TABLE temp_tokens ADD COLUMN first_download_at INTEGER DEFAULT 0")
	if err != nil {
		log.Printf("add temp_tokens.first_download_at skipped: %v", err)
	}
	if err := ensureFilesDownloadColumns(); err != nil {
		log.Fatalf("upgrade files table failed: %v", err)
	}
	if err := ensureFileReplicaTable(); err != nil {
		log.Fatalf("init file_replicas table failed: %v", err)
	}
	if err := ensureUploadSessionTable(); err != nil {
		log.Fatalf("init upload_sessions table failed: %v", err)
	}
}
func ensureFilesDownloadColumns() error {
	rows, err := db.Query("PRAGMA table_info(files)")
	if err != nil {
		return err
	}

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	rows.Close()

	if !columns["primary_backend"] {
		if _, err := db.Exec("ALTER TABLE files ADD COLUMN primary_backend TEXT DEFAULT 'local'"); err != nil {
			return err
		}
		columns["primary_backend"] = true
	}

	if !columns["size"] {
		log.Printf("闂傚倸鍊搁崐鎼佸磹瀹勬噴褰掑炊瑜滃ù鏍煏婵炵偓娅嗛柛銈呭閺屻倗绮欑捄銊ょ驳濠电偛鎳愭慨鍨┍婵犲洤围闁稿本鐭竟鏇㈡⒒娴ｉ涓茬紒韫矙閹ê顫濈捄铏诡唵闂佸憡渚楅崹閬嶅窗閸℃稒鐓曢柡鍥ュ妼娴滄粌鈹戦埄鍐ㄢ枙婵﹦绮幏鍛驳鐎ｎ偆绉烽柣搴ゎ潐濞叉﹢鏁嬮梺宕囩帛閺屻劑鍩ユ径濠庢僵妞ゆ挾鍋涚花銉モ攽閻愬瓨灏伴柛鈺佸暣瀹曟垿骞樼紒妯煎幍?size 闂傚倸鍊搁崐宄懊归崶顒夋晪鐟滃繘鍩€椤掍胶鈻撻柡鍛箘閸掓帒鈻庨幘宕囶唺濠碉紕鍋涢惃鐑藉磻閹捐绀冩い鏃傚帶閼板灝鈹戦悙鏉戠伇濡炲瓨鎮傚鏌ュ煛閸涱喖鈧敻鏌涜箛鎿冩Ц濞存粓绠栧娲川婵犲啫顦╅梺鍛婃尰閼归箖鍩㈠澶婂唨妞ゆ挾鍠撻崢浠嬫椤愩垺澶勬繛鍙夌墬缁傛帟顦归柡宀嬬節瀹曨亝鎷呴梹鎰晼濠电姰鍨奸～澶娒哄鍫濈闁绘顥嗛崷顓涘亾閿濆骸浜濋柡澶婃啞娣囧﹪鎮欓鍕ㄥ亾閺嵮屾綎濠电姵鑹鹃悡鏇㈡煕椤愮姴鍔氱痪鎯ф健閺岋繝宕橀妸褍顤€闂佺粯鎸诲ú鐔煎蓟閵娿儮鏀介柛鈩冾焽閵堚晠姊虹紒妯虹闁稿鎹囨俊?..")
		rows, err := db.Query("SELECT COUNT(*) FROM files")
		if err == nil {
			rows.Close()
			var count int
			db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
			if count > 0 {
				log.Printf("闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曚綅閸ヮ剦鏁冮柨鏇楀亾闁汇倗鍋撶换娑㈠幢濡闉嶉梺鎼炲€曠€氫即寮婚敓鐘查唶闁靛繆鍓濆В鍕⒑?%d 闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偞鐗犻、鏇氱秴濠㈣埖鍔栭弲鎼佹煟濡搫妫樼憸鏃堢嵁閺嶎偄鍨濋柣鐔告緲閳峰姊洪崫鍕紞濞存粠浜?..", count)
			}
		}
		if _, err := db.Exec("ALTER TABLE files RENAME TO files_old"); err != nil {
			log.Printf("闂傚倸鍊搁崐鎼佸磹閹间礁纾归柣鎴ｅГ閸婂潡鏌ㄩ弴姘舵濞存粌缍婇弻娑㈠箛閸忓摜鏁栭梺娲诲幗閹瑰洭寮婚悢铏圭＜闁靛繒濮甸悘鍫㈢磽娴ｅ搫啸濠电偐鍋撻梺鍝勮閸旀垿骞冮姀銈呬紶闁告洘鍨崹浠嬪箖瀹勬壋鏋庢繛鍡楁禋濞差參姊洪柅鐐茶嫰婢ь喚绱掗悩鑼х€规洘娲熼弻鍡楊吋閸℃ぞ缃曢梻浣告啞濞诧箓宕规导瀛樺€块柛顭戝亖娴滄粓鏌熼崫鍕棞濞存粓绠栧娲传閵夈儲鐝￠梺鎼炲妺閸楁娊銆佸璺何ㄩ柍杞拌兌椤︽澘顪冮妶鍡楃瑐闁煎啿澧庣划鏃傛崉娓氼垱瀵岄梺闈涚墕濡鎱ㄩ崒鐐寸厱? %v", err)
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT, 
			code TEXT UNIQUE, 
			name TEXT, 
			path TEXT, 
			size INTEGER,
			expire_at INTEGER, 
			hash TEXT,
			download_count INTEGER DEFAULT 0,
			download_limit INTEGER,
			primary_backend TEXT DEFAULT 'local'
		);`); err != nil {
			log.Printf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囨繝寰锋澘鈧洟宕导瀛樺剹婵炲棙鎸婚悡娆撴倵閻㈡鐒鹃崯鍝ョ磼閹冪稏缂侇喗鐟╁濠氭偄閻撳海顔夐梺閫涘嵆濞佳冣枔椤撶偐鏀介柍钘夋娴滄繈鏌ㄩ弴妯虹伈鐎殿喖顭烽幃銏☆槹鎼淬垺鐤傞梻浣烘嚀閸㈡煡鎯岄崼婵愮劷闁割偅娲橀埛鎴︽⒑椤愩倕浠滈柤娲诲灡閺呭爼顢氶埀顒勫蓟濞戙垺鍋嗗ù锝呮憸娴犫晛顪冮妶鍡樼８闁稿酣娼ч悾鐑芥偄绾拌鲸鏅㈡繛杈剧秬濞咃綀鍊存繝纰夌磿閸嬫垿宕愰弽顓炵闁硅揪绠戠壕? %v", err)
			return err
		}
		if _, err := db.Exec(`INSERT INTO files (id, code, name, path, size, expire_at, hash, download_count, download_limit, primary_backend)
			SELECT id, code, name, path, 0, expire_at, hash, COALESCE(download_count, 0), COALESCE(download_limit, 5), 'local' FROM files_old`); err != nil {
			log.Printf("闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曚綅閸ヮ剦鏁冮柨鏇楀亾闁汇倗鍋撶换娑㈠幢濡闉嶉梺鎼炲€曠€氫即寮婚敓鐘查唶闁靛繆鍓濆В鍕⒑閸濆嫬顏╅柛濠傜仢椤繒绱掑Ο璇差€撻梺鍏间航閸庮垶鍩€椤掆偓閸熸挳寮婚妶鍥ｅ亾閸︻厼孝闁绘挻鍔欓弻锟犲川閻楀牏銆愰柧缁樼墵閺屽秷顧侀柛鎾跺枛楠炲啯銈ｉ崘銊ョ€銈嗗姧缁蹭粙顢撻幘缁樷拺闁煎鍊曢弸鎴濐熆閻熺増顥犵紒顕嗙秮閹瑩鎮滃Ο鐓庡箰濠电姰鍨煎▔娑㈡晝閿旇姤娅犻梺顒€绉甸悡鏇㈡倵閿濆簼鎲炬俊鎻掑悑閵? %v", err)
			return err
		}
		if _, err := db.Exec("DROP TABLE files_old"); err != nil {
			log.Printf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囬梻浣虹帛閸旀洟顢氶鐘典笉濡わ絽鍟悡鍐喐濠婂牆绀堟慨妯块哺瀹曞弶绻涢幋娆忕仼鐎瑰憡绻冮妵鍕箻閸楃偟浠奸悗娈垮枙閸楁娊骞冨Δ鈧埢鎾诲垂椤旂晫褰梻浣告憸婵敻鎯勯婵囶棨闂備礁澹婇崑鍛哄鈧幃陇绠涘☉娆戝幈闂佹枼鏅涢崰姘枔閺冨牊鐓涢柛鈩冪懃娴犙囨煃瑜滈崜娆撴倶濮橆剦鐔嗘俊顖氱毞閸嬫挸顫濋悡搴＄濡炪値鍘煎ú鈺冪不濞戞ǚ妲堥弶鍫涘妽濞呭洭姊? %v", err)
		}
		log.Printf("files table migration completed")
	}

	return nil
}

// 缂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸绾惧綊鏌ｉ幋锝呅撻柛銈呭閺屾盯顢曢敐鍡欘槬缂備焦鍔栭〃濠囧蓟閿熺姴鐐婇柕澶堝劤閸旀挳姊烘潪鎵槮婵炲樊鍙冨濠氭偄閸忓皷鎷婚柣搴ｆ暩椤牊淇婃禒瀣拺缂佸娼￠妤呮倵濞戞帗娅婂┑锛勬暬瀹曠喖顢涘杈╂綁闂備焦鎮堕崕婊堝磼濞戞碍缍庣紓鍌氬€搁崐椋庣矆娓氣偓钘濇い鏍ㄥ嚬閻掍粙鏌ㄩ悢鍝勑㈤柦鍐枑缁绘盯骞嬮悜鍡欏姱闂佸搫鍟悧鍡欑矆閸愵喗鐓忓┑鐘插暙婵倿鏌涙惔锝嗘毈鐎殿喖顭烽崺鍕礃閳轰緡鈧捇姊婚崒姘卞缂佸鐗撳畷鎴﹀箻濠㈠嫭妫冨畷姗€濡搁妷褌鍠婂┑鐘愁問閸犳鏁冮埡鍛？妞ゅ繐濞婂ú顏嶆晜鐎广儱妫欏▍鍡涙⒒娴ｅ憡鍟炵紒顔肩焸瀹曨垱瀵奸弶鎴Ｐ?
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func cancelMarkerDir() string {
	return filepath.Join(globalCfg.TempDir, ".cancelled")
}

func cancelMarkerPath(uploadID string) string {
	return filepath.Join(cancelMarkerDir(), uploadID)
}

func activeMarkerPath(uploadID string) string {
	return filepath.Join(globalCfg.TempDir, uploadID, ".active")
}

func markUploadCanceled(uploadID string) error {
	if err := os.MkdirAll(cancelMarkerDir(), os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(cancelMarkerPath(uploadID), []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0666)
}

func isUploadCanceled(uploadID string) bool {
	_, err := os.Stat(cancelMarkerPath(uploadID))
	return err == nil
}

func touchUploadActive(uploadID string) error {
	return os.WriteFile(activeMarkerPath(uploadID), []byte{}, 0666)
}

func removeUploadChunks(uploadID string) error {
	return os.RemoveAll(filepath.Join(globalCfg.TempDir, uploadID))
}

func maybeCleanupStaleTempUploads(currentUploadID string) {
	if !shouldRunTempCleanup() {
		return
	}

	entries, err := os.ReadDir(globalCfg.TempDir)
	if err != nil {
		log.Printf("闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌℃径瀣婵炲樊浜滈悡娑㈡煕濠娾偓閻掞箓寮查鍫熷仭婵犲﹤瀚悘鏉戔攽閿涘嫭鏆鐐叉喘瀵墎鎹勯妸銉㈠亾閻愮儤鈷戦梻鍫熶腹濞戙垹鐒垫い鎺戝閸戠娀鏌″搴″箺闁绘挾鍠栭獮鏍庨鈧悘顕€鏌嶉娑欑闁归攱鍨跺蹇涘Ω閿濆嫮鐩庨梺鎸庣矊椤嘲鐣烽崼鏇炍╃憸宥夌嵁濡ゅ懏鈷掑ù锝囶焾閺嗛亶鏌熺喊鍗炰喊闁轰礁鍟存俊鑸靛緞婵犲倸娈ゆ繝鐢靛仦閸ㄨ泛顫濋妸褍顥氶柣锝呭閸嬫捇鐛崹顔煎闂佺娅曢崝鏍偋鎼淬劍鐓熼幖杈剧磿娴犳稒绻濋姀鈽嗙劷闁逞屽墯閸戝綊宕㈡總绯曗偓锕傚炊椤忓棛鏉稿┑鐐村灱妞存悂寮插┑瀣拺? %v", err)
		return
	}

	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		uploadID := entry.Name()
		if uploadID == ".cancelled" || uploadID == currentUploadID {
			continue
		}

		uploadPath := filepath.Join(globalCfg.TempDir, uploadID)
		shouldDelete := isUploadCanceled(uploadID)
		if !shouldDelete {
			lastActiveAt, err := uploadLastActiveAt(uploadID, uploadPath)
			if err != nil {
				log.Printf("闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌℃径瀣婵炲樊浜滈悡娑㈡煕濠娾偓閻掞箓寮查鍫熷仭婵犲﹤瀚悘鏉戔攽閿涘嫭鏆鐐叉喘瀵墎鎹勯妸銉㈠亾閻愮儤鈷戦梻鍫熶腹濞戙垹鐒垫い鎺戝閸戠娀鏌″鍐ㄥ缂佲檧鍋撻梻浣圭湽閸ㄨ棄顭囪閻剟姊绘担鍝勫付闁哥喎娼￠幃銉︾附缁嬭法鐣哄┑鐐叉閸旀寮ч埀顒勬⒑閸涘﹤濮﹀ù婊呭仱椤㈡瑩寮撮姀鈾€鎷洪梺鍛婄☉閿曪箓骞婇崘鈹夸簻闁挎柨鐏濆畵鍡涙煥濠靛牆浠滈摶鏍煕閹板吀绨介柨娑欑矒閹宕楁径濠佸濠电姷鏁告慨鎾磹閹间礁鐓曢悗锝庡枟閸婂灚绻涢崼婵堜虎婵炲懏锕㈤弻娑㈡晲韫囨洖鍩岄梺浼欑秮閺€杈╃紦閻ｅ瞼鐭欓柛顭戝枛缁侇噣姊绘担铏瑰笡婵炲弶鐗犲畷鎰節濮橆剝袝闂佽崵鍠愭竟瀣绩娴犲鐓熸慨妤€妫楅弸娑㈡煟韫囷絼閭柡灞剧⊕閹棃濡堕崨顒佺潖闂備礁鎲＄敮妤冩暜閳ュ磭鏆︽繝濠傛－濡插ジ姊? %v", err)
				continue
			}
			shouldDelete = now.Sub(lastActiveAt) >= tempUploadStaleAfter
		}

		if !shouldDelete {
			continue
		}

		if err := os.RemoveAll(uploadPath); err != nil {
			log.Printf("婵犵數濮烽弫鍛婃叏閻戣棄鏋侀柟闂寸绾惧鏌ｉ幇顒佹儓缂佺姳鍗抽弻鐔兼⒒鐎靛壊妲紓浣哄Х婵炩偓闁哄瞼鍠栭幃褔宕奸悢鍝勫殥缂傚倷鑳舵慨鐢告偋閺囥垹鐓橀柟杈鹃檮閸嬫劙鏌熺紒妯虹瑲婵炲牜鍘剧槐鎾存媴缁嬫鏆㈤梺绋款儍閸婃洟顢氶敐澶婇唶闁哄洨鍋ら崬璺衡攽閻愭潙鐏﹂柨姘亜椤掆偓椤﹂潧顫忓ú顏咁棃婵炴垶鐟ョ粣娑㈡⒑閸濄儱孝闂佸府缍佸畷娲Ψ閿曗偓缁剁偤鏌熼柇锕€澧版い鏃€甯炵槐鎾存媴閸︻収鐏卞銈庡亜椤︾敻濡撮崒鐐村殐闁冲搫鍟伴敍婵囩箾鏉堝墽鎮兼い顓炵墦閹虫粏銇愰幒鎾跺幗闂佸搫璇炴担闀愬垝婵°倗濮烽崑娑樜涘┑鍡欐殾闁绘梻鈷堥弫宥夋煥濠靛棙顥炴繛鍛崌濮婄粯鎷呯粙鎸庡€紓浣风劍閹稿啿顕ｉ幓鎺嗘斀闁糕剝顨堥ˇ褎绻濋悽闈浶ラ柡浣告啞閹便劍瀵奸弶鎴濈€┑鐐叉▕娴滄粎澹曢崸妤佺厾婵炴潙顑嗗▍鍛存煕閵婏妇绠為柡灞剧洴楠炴ê螖閳ь剛鈧凹鍓熷? %v", err)
			continue
		}
		if err := os.Remove(cancelMarkerPath(uploadID)); err != nil && !os.IsNotExist(err) {
			log.Printf("婵犵數濮烽弫鍛婃叏閻戣棄鏋侀柟闂寸绾惧鏌ｉ幇顒佹儓缂佺姳鍗抽弻鐔兼⒒鐎靛壊妲紓浣哄Х婵炩偓闁哄瞼鍠栭幃褔宕奸悢鍝勫殥缂傚倷鑳舵慨鐢告偋閺囥垹鐓橀柟杈鹃檮閸嬫劙鏌熺紒妯虹瑲婵炲牆鐖煎鍝勭暦閸モ晛绗￠梺鍝勮閸旀垿宕洪悙鍝勭闁挎棁妫勬禍褰掓煛婢跺﹦澧遍柛瀣瀹曘垽鏁撻悩鏂ユ嫼缂傚倷鐒﹂敋濠殿喖娲弻銊╁即閵娿倝鍋楅悗娈垮枦椤曆囧煡婢跺á鐔兼煥鐎ｎ兘鍋撴繝姘拺鐟滅増甯掓禍浼存煕閻樺磭澧甸柨婵堝仩椤﹀绱掓潏銊﹀鞍闁瑰嘲鎳樺畷婊堝矗婢诡厹鍔戝铏圭矙鐠恒劎顔夐梺鎸庣閵囧嫰寮撮鍡櫳戠紓浣稿€圭敮鐐哄焵椤掑﹦绉靛ù婊呭仱瀹曟劙鎮欏顔藉瘜闂侀潧鐗嗛幊鎰不閹殿喚纾煎璺侯儐鐏忥妇鈧鍣崑鍡涘箟閹绢喖绀嬫い鎾跺Х濡插洭姊绘笟鈧褏鎹㈤崱娑樼劦妞ゆ巻鍋撻柛鐔稿閹便劑宕妷褏锛濇繛鎾磋壘濞层倝寮稿☉姗嗙唵鐟滃瞼鍒掑▎鎾跺祦? %v", err)
		}
	}

	cleanupCancelMarkers(now)
}

func shouldRunTempCleanup() bool {
	tempCleanupMu.Lock()
	defer tempCleanupMu.Unlock()

	now := time.Now()
	if !lastTempCleanupAt.IsZero() && now.Sub(lastTempCleanupAt) < tempCleanupMinInterval {
		return false
	}
	lastTempCleanupAt = now
	return true
}

func uploadLastActiveAt(uploadID, uploadPath string) (time.Time, error) {
	activeInfo, err := os.Stat(activeMarkerPath(uploadID))
	if err == nil {
		return activeInfo.ModTime(), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return time.Time{}, err
	}

	dirInfo, err := os.Stat(uploadPath)
	if err != nil {
		return time.Time{}, err
	}
	return dirInfo.ModTime(), nil
}

func cleanupCancelMarkers(now time.Time) {
	entries, err := os.ReadDir(cancelMarkerDir())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌℃径瀣婵炲樊浜滈悡娑㈡煕濠娾偓閻掞箓寮查鍫熷仭婵犲﹤瀚悘鏉戔攽閿涘嫭鏆鐐叉喘瀵墎鎹勯妸銉㈠亾閻愮儤鈷戦梻鍫熶腹濞戙垹鐒垫い鎺戝閸戠娀鏌″鍐ㄥ缂佲檧鍋撻梻浣圭湽閸ㄨ棄顭囪閻剟姊绘担鍝勫付闁哥喎娼￠幃銉︾附缁嬭法鐣哄┑鐐叉閸旀寮ч埀顒勬⒑閹肩偛鍔橀柛鏂块叄瀹曘垽宕￠悙鈺傛杸闂佺粯鍔曞鍫曀夐悙鐑樼厱闁哄啠鍋撴い銊ユ瀵煡宕奸弴銊︽櫌闂佺鏈划搴ㄦ晬濠婂嫮绡€闁靛骏绲剧涵楣冩煠濞茶鐏︾€规洏鍨婚埀顒傛暩绾爼宕戦幘鏂ユ灁闁割煈鍠楅悘鍡欑磽娓氬洤鏋熼柣鐔村劦閹箖鎮滈挊澶愬敹闂佸搫娲㈤崝宥夊疾濠婂牊鍊垫鐐茬仢閸旀岸鏌熼崘鑼鐎殿喗濞婇、鏃堝醇閻斿弶瀚藉┑鐐舵彧缁蹭粙骞栭锕€绀夐柨鏇炲€归悡娑㈡煕濞戝彉绨兼繛鍛功閳ь剝顫夊ú鎴﹀础閸愬樊鍤曞ù鐘差儛閺佸啴鏌曢崼婵囩ォ婵″弶鎮傞弻锝嗘償閿涘嫮鏆涢梺绋块瀹曨剝鐏嬪┑掳鍊曢崯顖烆敃娴犲鐓熼柟閭﹀墯閹牏绱掗悪娆忔处閻撴洘銇勯幇闈涗簻濞存粍绮岄湁婵犲﹤楠搁悘锝夋煙? %v", err)
		}
		return
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Printf("闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌℃径瀣婵炲樊浜滈悡娑㈡煕濠娾偓閻掞箓寮查鍫熷仭婵犲﹤瀚悘鏉戔攽閿涘嫭鏆鐐叉喘瀵墎鎹勯妸銉㈠亾閻愮儤鈷戦梻鍫熶腹濞戙垹鐒垫い鎺戝閸戠娀鏌″鍐ㄥ缂佲檧鍋撻梻浣圭湽閸ㄨ棄顭囪閻剟姊绘担鍝勫付闁哥喎娼￠幃銉︾附缁嬭法鐣哄┑鐐叉閸旀寮ч埀顒勬⒑閹肩偛鍔橀柛鏂块叄瀹曘垽宕￠悙鈺傛杸闂佺粯鍔曞鍫曀夐悙鐑樼厱闁哄啠鍋撴い銊ユ瀵煡宕奸弴銊︽櫌闂佺鏈划搴ㄦ晬濠婂嫮绡€闁靛骏绲剧涵楣冩煠濞茶鐏︾€规洏鍨婚埀顒傛暩绾爼宕戦幘鏂ユ灁闁割煈鍠楅悘鍡欑磽娓氬洤鏋熼柣鐔村劦閹箖鎮滈挊澶愬敹闂佸搫娲㈤崝宥夊疾濠婂牊鍊垫鐐茬仢閸旀岸鏌熼崘鑼鐎殿喗濞婇、鏃堝礋閵婏附鏉搁梻浣虹帛閸ㄩ潧煤閵娾晛绀嗛柣妤€鐗忕粻楣冩煠绾板崬澧柍璇茬墢閳ь剝顫夊ú妯煎垝瀹€鍕厴闁硅揪绠戦悙濠勬喐濠婂牆鍚归柕鍫濐槹閳锋垿鏌涘☉姗堟缂佸爼浜堕弻娑㈡偐瀹曞洤鈷堝┑鐐叉閸ㄤ粙骞冨▎鎾充紶闁告洦鍙冮悰鎾绘⒒娴ｇ顥忛柛瀣浮瀹曟垿宕熼鍌ゆ锤濡炪倖鐗楃划宥夊磻? %v", err)
			continue
		}
		if now.Sub(info.ModTime()) < cancelMarkerMaxAge {
			continue
		}
		if err := os.Remove(filepath.Join(cancelMarkerDir(), entry.Name())); err != nil && !os.IsNotExist(err) {
			log.Printf("闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偛顦甸弫宥夊礋椤掍焦顔囬梻浣虹帛閸旀洟顢氶鐘典笉濡わ絽鍟悡鍐喐濠婂牆绀堟慨妯块哺瀹曞弶绻涢幋娆忕仼鐎瑰憡绻冮妵鍕箻鐠哄搫澹夐梺鍛婃煥椤︻垶鍩為幋锔藉€烽梻鍫熺☉娴犳ê顪冮妶搴″箹闁搞垺鐓￠幃楣冩倻閽樺鍞堕梺鍝勬川閸婏綁鍩￠崨顔惧弳闂佸搫娲﹂〃鍛妤ｅ啯鈷戠紓浣姑粭鎺楁煙鐠囇呯？闁瑰箍鍨归埥澶愬閻樻鍚呴梻浣虹帛閸旀牕顭囧▎鎴濇瀳鐎广儱顦伴埛鎴︽煙缁嬪灝顒㈢紒宀冩硶缁辨帡骞撻幒鏂捐檸闁告浜堕弻鐔兼偋閸喓鍑￠梻浣稿船濞差參寮婚敓鐘茬倞闁宠桨妞掗幋鐑芥⒑缂佹ɑ灏伴柛銊ョ仢椤繐煤椤忓嫪绱堕梺鍛婃处閸樻粓鎮╃紒妯煎幈闂佸搫鍊稿锟狀敁濡ゅ懏鐓涢柛鈥崇箲濞呭﹥鎱ㄦ繝鍕笡闁瑰嘲鎳樺畷銊︾節閸愩劌澹嶇紓鍌氬€风粈渚€藝閺夋鐒芥繛鍡樺灥瀵煡姊绘担绋挎毐闁圭⒈鍋婇獮蹇曗偓锝庡枛閺嬩礁鈹戦悩鍙夊闁绘挻娲熼弻鏇熷緞濞戞艾鍩屾繝寰枫倕浜圭紒杈ㄥ浮瀹曟粍鎷呴梹鎰潟闁诲孩顔栭崰鏍偉閻撳海鏆﹀┑鍌氬閺佸啯銇勯顐㈠箹闁搞倝浜堕弻? %v", err)
		}
	}
}

// 闂傚倸鍊搁崐宄懊归崶顒夋晪鐟滃秹锝炲┑瀣櫇闁稿矉濡囩粙蹇旂節閵忥絾纭鹃柤娲诲灦瀵悂宕奸埗鈺佷壕妤犵偛鐏濋崝姘舵煙瀹勯偊鍎忛柕鍡樺笚缁绘繂顫濋鐘插箞闂佽绻掗崑娑欐櫠娴犲违闁圭儤鎸舵禍婊堟煃閸濆嫬鏆欓柛妯绘尦閺岀喖顢欓懖鈺冃ㄩ悗瑙勬礀閻栧吋淇婇幖浣肝ㄧ憸蹇涘Χ闁垮绻嗛柣鎰典簻閳ь兙鍊濆畷鎴炵瑹閳ь剙鐣烽幇顑芥斀閻庯綆浜ｉ幗鏇㈡⒑闂堟侗鐒鹃柛搴枛鍗遍柛顐ゅ枔缁犻箖鏌涢埄鍏狀亞绮幒鎾变簻闁靛濡囩粻鐐存叏婵犲啯銇濇鐐寸墵閹瑩骞撻幒婵堚偓宕囩磽閸屾艾鈧摜绮旈幘顔芥櫇闁靛牆顧€缂嶆牠鐓崶銊︹拻妞も晝鍏橀幃妤呮晲鎼存ê浜炬い鎾筹工濞诧箓鎮￠弴銏＄厪濠㈣泛鐗嗘俊濂告煟閹炬潙濮嶉柡灞界Ч閺屻劎鈧綆鍓欓埛灞解攽椤旂》鏀绘俊鐐扮矙楠炲啫顭ㄩ崼婵堝幐闂佺鏈粙鎾广亹閸℃稒鈷戞繛鑼额唺缁ㄧ粯銇勯敃浣峰惈婵″弶鍔欓獮鎺楀箠閾忣偅顥堥柛鈹惧亾濡炪倖甯掔€氼剛澹曡ぐ鎺撶厱鐟滃酣銆冮崨顖滀笉婵鍩栭悡鏇㈡煙娴煎瓨娑ч柡瀣〒缁辨帡鍩€椤掑倵鍋撻敐搴′簴濞存粍绮撻弻锟犲磼濮樺彉铏庨梺鎶芥敱濡啴寮婚敐澶嬫櫜闁糕剝菧娴犮垹鈹戦纭锋敾婵＄偠妫勯悾鐑筋敃閿曗偓缁€瀣亜閹捐泛浜归柛娆欑節濮婄粯鎷呴崨濠冨枑婵犳鍠氶弫璇茬暦濠靛棌鏋庨煫鍥风到濞堛劑姊鸿ぐ鎺擄紵缂佲偓娴ｈ櫣鐭嗛柛鎰靛枟閻撳啴鏌涘┑鍡楊仼闁哄鍊栫换娑㈠级閹存繍浼冮梺鍝勭焿缂嶄線骞冮埡鍛煑濠㈣泛顑呴崜鐢告⒒娴ｅ憡璐￠柍宄扮墦瀹曟垶绻濋崶鈺佺ウ?DB 闂傚倸鍊搁崐宄懊归崶褏鏆﹂柛顭戝亝閸欏繘鏌熼幆鏉啃撻柛濠傛健閺屻劑寮村Δ鈧禍鎯ь渻閵堝骸骞栭柣蹇旂箚閻忔帡姊洪崗鑲┿偞闁哄懏绻堣棟?
func startCleanupTask() {
	ticker := time.NewTicker(time.Duration(globalCfg.RetentionInterval) * time.Minute)
	cleanupExpiredFilesOnce()
	cleanupStaleUploadSessionsOnce()
	for range ticker.C {
		cleanupExpiredFilesOnce()
		cleanupStaleUploadSessionsOnce()
	}
}

func cleanupExpiredFilesOnce() {
	now := time.Now().Unix()
	nowText := time.Unix(now, 0).Format("2006-01-02 15:04:05")
	rows, err := db.Query("SELECT id, code, name, path, expire_at FROM files WHERE expire_at < ?", now)
	if err != nil {
		log.Printf("query expired files failed: now=%d (%s), err=%v", now, nowText, err)
		return
	}
	type expiredFile struct {
		id       int
		code     string
		name     string
		path     string
		expireAt int64
	}
	var expiredFiles []expiredFile
	for rows.Next() {
		var expired expiredFile
		if err := rows.Scan(&expired.id, &expired.code, &expired.name, &expired.path, &expired.expireAt); err != nil {
			log.Printf("scan expired file row failed: now=%d (%s), err=%v", now, nowText, err)
			continue
		}
		expiredFiles = append(expiredFiles, expired)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate expired files failed: now=%d (%s), err=%v", now, nowText, err)
	}
	rows.Close()
	if len(expiredFiles) == 0 {
		log.Printf("expired file cleanup check complete: now=%d (%s), expired=0", now, nowText)
		return
	}
	log.Printf("expired file cleanup check complete: now=%d (%s), expired=%d", now, nowText, len(expiredFiles))
	for _, expired := range expiredFiles {
		expireText := time.Unix(expired.expireAt, 0).Format("2006-01-02 15:04:05")
		log.Printf(
			"deleting expired file: id=%d, code=%s, name=%s, path=%s, expireAt=%d (%s), now=%d (%s)",
			expired.id,
			expired.code,
			expired.name,
			expired.path,
			expired.expireAt,
			expireText,
			now,
			nowText,
		)
		if strings.TrimSpace(expired.path) != "" {
			if err := os.Remove(expired.path); err != nil && !os.IsNotExist(err) {
				log.Printf("remove expired file failed: id=%d, path=%s, err=%v", expired.id, expired.path, err)
			}
		}
		deleteFileRecordByCode(expired.code)
	}
}

// random string helper
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("闂傚倸鍊搁崐鎼佸磹閻戣姤鍊块柨鏇炲€归崕鎴犳喐閻楀牆绗掗柛銊ュ€搁埞鎴︽偐鐎圭姴顥濈紓浣瑰姈椤ㄥ﹪寮婚悢鍏煎亱闁割偆鍠撻崙锟犳⒑閹肩偛濡奸柛濠傜秺楠炲牓濡搁妷搴ｅ枛瀹曞綊顢欓幆褍缂氶梻鍌欑劍婵炲﹪寮ㄩ崡鐏绘椽鎮㈤崗鐓庝簵濡炪倖鍔х粻鎴︽倷婵犲洦鐓忓┑鐘茬箺閸氬倿鏌熷畡鐗堣础缂佽鲸鎸婚幏鍛存濞戞﹩鐎撮柣搴″帨閸嬫捇鏌熼梻瀵割槮闁藉啰鍠栭弻銊モ攽閸♀晜鈻撻梺杞扮閿曨亪寮婚妶鍡樺弿闁归偊鍏橀崑鎾诲Χ閸涘偊缍侀、姘跺焵椤掆偓椤繐煤椤忓拋妫冨┑鐐村灦閻熴儵藝閳哄懏鈷戦柟鑲╁仜婵¤偐绱撳鍜冭含闁? %v", err)
	}
	for i := range b {
		b[i] = letters[b[i]%byte(len(letters))]
	}
	return string(b)
}

func isValidUploadID(uploadID string) bool {
	_, err := uuid.Parse(uploadID)
	return err == nil
}

func normalizeOrigin(rawOrigin string) string {
	rawOrigin = strings.TrimSpace(rawOrigin)
	if rawOrigin == "" {
		return ""
	}

	parsed, err := url.Parse(rawOrigin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(rawOrigin, "/")
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isValidVersion(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func readStoredVersion() string {
	defaultVersion := defaultVersionPrefix + ".0"

	data, err := os.ReadFile("version.txt")
	if err != nil {
		return defaultVersion
	}

	version := normalizeStoredVersion(string(data))
	if !isValidVersion(version) {
		return defaultVersion
	}
	return version
}

func shouldAutoBumpVersion(exePath string) bool {
	normalized := strings.ToLower(filepath.ToSlash(exePath))
	return !strings.Contains(normalized, "/go-build")
}

func shouldBumpVersionForExecutable(exePath string) (bool, error) {
	exeInfo, err := os.Stat(exePath)
	if err != nil {
		return false, err
	}

	versionInfo, err := os.Stat("version.txt")
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	return exeInfo.ModTime().After(versionInfo.ModTime()), nil
}

func nextVersion(currentVersion string) string {
	if !isValidVersion(currentVersion) {
		return defaultVersionPrefix + ".0"
	}

	parts := strings.Split(currentVersion, ".")
	currentPrefix := parts[0] + "." + parts[1]
	if currentPrefix != defaultVersionPrefix {
		return defaultVersionPrefix + ".0"
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return defaultVersionPrefix + ".0"
	}

	return fmt.Sprintf("%s.%d", defaultVersionPrefix, patch+1)
}

func normalizeStoredVersion(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(raw, "\uFEFF"))
}

func isValidFileHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func sanitizeFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	fileName = strings.ReplaceAll(fileName, "\\", "/")
	fileName = path.Base(fileName)
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || fileName == "." || fileName == ".." {
		return "file"
	}

	fileName = strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r == 127:
			return -1
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		default:
			return r
		}
	}, fileName)
	fileName = strings.Trim(fileName, ". ")
	if fileName == "" {
		return "file"
	}
	return limitFileNameBytes(fileName, maxFileNameLen)
}

func limitFileNameBytes(fileName string, maxBytes int) string {
	if len(fileName) <= maxBytes {
		return fileName
	}

	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	limit := maxBytes - len(ext)
	if limit < 1 {
		ext = ""
		base = fileName
		limit = maxBytes
	}

	var builder strings.Builder
	for _, r := range base {
		rText := string(r)
		if builder.Len()+len(rText) > limit {
			break
		}
		builder.WriteString(rText)
	}

	result := strings.Trim(builder.String()+ext, ". ")
	if result == "" {
		return "file"
	}
	return result
}

func closeAndRemove(file *os.File, path string) {
	if err := file.Close(); err != nil {
		log.Printf("close file failed: %v", err)
	}
	if err := os.Remove(path); err != nil {
		log.Printf("remove file failed: %v", err)
	}
}

func deleteFileRecordByCode(code string) {
	if r2Replicator != nil {
		if err := r2Replicator.abortAndDeleteMultipartState(code, replicaBackendR2); err != nil {
			log.Printf("abort replica multipart state before deleting file record failed: code=%s, err=%v", code, err)
		}
	} else {
		if err := deleteFileReplicaMultipartState(code, replicaBackendR2); err != nil {
			log.Printf("delete replica multipart state before deleting file record failed: code=%s, err=%v", code, err)
		}
	}
	if err := deleteUploadedR2ObjectForCode(code); err != nil {
		log.Printf("delete uploaded R2 object before deleting file record failed: code=%s, err=%v", code, err)
	}
	deleteFileReplicaRecordsByCode(code)
	if _, err := db.Exec("DELETE FROM files WHERE code = ?", code); err != nil {
		log.Printf("delete file record failed: %v", err)
	}
}

func deleteFileAndRecordByCode(code, filePath string) {
	if strings.TrimSpace(filePath) != "" {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("remove local file failed: path=%s, err=%v", filePath, err)
		}
	}
	deleteFileRecordByCode(code)
}

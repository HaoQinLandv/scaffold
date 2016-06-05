package commands

import (
	"database/sql"
	"errors"
	"fmt"
	"go/build"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
	_ "github.com/go-sql-driver/mysql"
	"github.com/liujianping/scaffold/symbol"
)

func src_template_dir(ctx *cli.Context) (string, error) {
	template_folder_dir := ctx.GlobalString("template-folder")
	if template_folder_dir != "" {
		return template_folder_dir, nil
	}

	template := ctx.GlobalString("template")
	gopath := build.Default.GOPATH
	if gopath == "" {
		return "", errors.New("Abort: GOPATH environment variable is not set. ")
	}
	return path.Join(filepath.SplitList(gopath)[0], "src", "github.com/liujianping/scaffold", "templates", template), nil
}

func dest_project_dir(project string) (string, error) {
	gopath := build.Default.GOPATH
	if gopath == "" {
		return "", errors.New("Abort: GOPATH environment variable is not set. ")
	}

	return path.Join(filepath.SplitList(gopath)[0], "src", project), nil
}

func hasSuffix(fpath string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(fpath, "."+strings.TrimPrefix(suffix, ".")) {
			return true
		}
	}
	return false
}

type Option struct {
	Name        string
	Code        string
	OptionName  string
	OptionCode  string
	OptionValue int64
}

func system_options(db *sql.DB) (map[string][]Option, error) {
	options := make(map[string][]Option)

	rows, err := db.Query(`SELECT name, code, option_code, option_value , option_name FROM options`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var opt Option
		if err := rows.Scan(&opt.Name, &opt.Code, &opt.OptionCode, &opt.OptionValue, &opt.OptionName); err != nil {
			log.Println("scan failed:", err)
			return nil, err
		}

		if sel, ok := options[opt.Code]; ok {
			sel = append(sel, opt)
			options[opt.Code] = sel
		} else {
			sel = []Option{}
			sel = append(sel, opt)
			options[opt.Code] = sel
		}
	}

	return options, nil
}

func Generate(ctx *cli.Context) {
	template_dir, err := src_template_dir(ctx)
	if err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}
	if template_dir == "" {
		fmt.Println("scaffold generate failed: none templates")
		return
	}

	args := ctx.Args()
	if len(args) != 1 {
		fmt.Println("Usage: scaffold generate <project/path>")
		return
	}

	project := args[0]
	project_dir, err := dest_project_dir(project)
	if err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}

	suffixes := ctx.GlobalStringSlice("template-suffix")
	ignores := ctx.GlobalStringSlice("ignore-suffix")
	force := ctx.GlobalBool("force")
	driver := ctx.String("driver")
	database := ctx.String("database")
	host := ctx.String("host")
	port := ctx.Int("port")
	user := ctx.String("username")
	pass := ctx.String("password")

	dsn := symbol.DSNFormat(host, port, user, pass, database)
	db, err := sql.Open(driver, dsn)
	if err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}
	defer db.Close()

	tables, err := symbol.GetAllTables(db)
	if err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}
	options, err := system_options(db)
	if err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}

	if err := filepath.Walk(template_dir, func(srcPath string, info os.FileInfo, err error) error {
		data := map[string]interface{}{
			"project": project,
			"tables":  tables,
			"options": options,
		}
		for i, table := range tables {
			data["table"] = table
			data["index"] = i

			pathName, err := symbol.RenderString(srcPath, data)
			if err != nil {
				return err
			}

			destPath := path.Join(project_dir, strings.TrimPrefix(pathName, template_dir))

			if info.IsDir() {
				if err := os.MkdirAll(destPath, info.Mode()); err != nil {
					return err
				}
			} else {
				if hasSuffix(pathName, ignores) {
					break
				} else if hasSuffix(pathName, suffixes) {
					if err := symbol.RenderTemplate(srcPath, destPath, data, force); err != nil {
						return err
					}
				} else {
					if err := symbol.CopyFile(srcPath, destPath, force); err != nil {
						return err
					}
				}
			}

			if srcPath == pathName {
				break
			}
		}
		return nil
	}); err != nil {
		fmt.Println("scaffold generate failed:", err)
		return
	}
	if len(suffixes) == 0 {
		fmt.Println("WARNING: scaffold template suffix is unset, just copy files.")
	}
}

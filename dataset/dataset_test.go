package dataset

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadFiles_JSONL(test *testing.T) {
	Convey("Given a local JSONL file", test, func() {
		dir := test.TempDir()
		path := filepath.Join(dir, "train.jsonl")

		err := os.WriteFile(path, []byte(
			`{"text":"hello"}`+"\n"+
				`{"text":"world"}`+"\n",
		), 0o644)
		So(err, ShouldBeNil)

		dataset, err := LoadFiles(context.Background(), "json", map[string][]string{
			"train": {path},
		}, "train")

		So(err, ShouldBeNil)

		Convey("It should stream rows without caching", func() {
			rows := collectRows(test, dataset.Take(1))

			So(rows, ShouldHaveLength, 1)
			So(rows[0]["text"], ShouldEqual, "hello")
		})

		Convey("It should support skip and take", func() {
			rows := collectRows(test, dataset.Skip(1).Take(1))

			So(rows, ShouldHaveLength, 1)
			So(rows[0]["text"], ShouldEqual, "world")
		})

		Convey("It should project columns", func() {
			rows := collectRows(test, dataset.SelectColumns("text").Take(1))

			So(rows, ShouldHaveLength, 1)
			So(rows[0], ShouldResemble, Row{"text": "hello"})
		})
	})
}

func TestLoadFiles_CSV(test *testing.T) {
	Convey("Given a local CSV file", test, func() {
		dir := test.TempDir()
		path := filepath.Join(dir, "train.csv")

		err := os.WriteFile(path, []byte("label,text\n1,hello\n0,world\n"), 0o644)
		So(err, ShouldBeNil)

		dataset, err := LoadFiles(context.Background(), "csv", map[string][]string{
			"train": {path},
		}, "train")
		So(err, ShouldBeNil)

		rows := collectRows(test, dataset)

		So(rows, ShouldHaveLength, 2)
		So(rows[0]["label"], ShouldEqual, "1")
		So(rows[0]["text"], ShouldEqual, "hello")
	})
}

func TestIterableDataset_ShuffleAndFilter(test *testing.T) {
	Convey("Given a small local dataset", test, func() {
		dir := test.TempDir()
		path := filepath.Join(dir, "train.jsonl")

		err := os.WriteFile(path, []byte(
			`{"label":0}`+"\n"+
				`{"label":1}`+"\n"+
				`{"label":2}`+"\n",
		), 0o644)
		So(err, ShouldBeNil)

		dataset, err := LoadFiles(context.Background(), "json", map[string][]string{
			"train": {path},
		}, "train")
		So(err, ShouldBeNil)

		filtered := dataset.Filter(func(row Row) (bool, error) {
			label, _ := row["label"].(float64)
			return label >= 1, nil
		})

		rows := collectRows(test, filtered)

		So(rows, ShouldHaveLength, 2)
	})
}

func TestIterableDataset_Shard(test *testing.T) {
	Convey("Given multiple local JSONL shards", test, func() {
		dir := test.TempDir()
		first := filepath.Join(dir, "000.jsonl")
		second := filepath.Join(dir, "001.jsonl")

		So(os.WriteFile(first, []byte(`{"id":1}`+"\n"), 0o644), ShouldBeNil)
		So(os.WriteFile(second, []byte(`{"id":2}`+"\n"), 0o644), ShouldBeNil)

		dataset, err := LoadFiles(context.Background(), "json", map[string][]string{
			"train": {first, second},
		}, "train")
		So(err, ShouldBeNil)
		So(dataset.NumShards(), ShouldEqual, 2)

		shard, err := dataset.Shard(2, 1)

		So(err, ShouldBeNil)

		rows := collectRows(test, shard)

		So(rows, ShouldHaveLength, 1)
		So(rows[0]["id"], ShouldEqual, float64(2))
	})
}

func collectRows(test testing.TB, dataset *IterableDataset) []Row {
	test.Helper()

	rows := make([]Row, 0)

	for row, err := range dataset.All(context.Background()) {
		if err != nil {
			test.Fatalf("iterate dataset: %v", err)
		}

		rows = append(rows, row)
	}

	return rows
}

package gosseract

import (
	"io"
	"io/ioutil"
	"os"
	"testing"
)

func BenchmarkClient_New(b *testing.B) {
	client := NewClient()
	client.Close()

	for i := 0; i < b.N; i++ {
		client := NewClient()
		client.Close()
	}
}

func BenchmarkClient_Text(b *testing.B) {
	for i := 0; i < b.N; i++ {
		client := NewClient()
		client.SetImage("./test/data/001-helloworld.png")
		client.Text()
		client.Close()
	}
}

func BenchmarkClient_Text2(b *testing.B) {
	client := NewClient()
	for i := 0; i < b.N; i++ {
		client.SetImage("./test/data/001-helloworld.png")
		client.Text()
	}
	client.Close()
}

func BenchmarkClient_Text3(b *testing.B) {
	client := NewClient()
	for i := 0; i < b.N; i++ {
		file, _ := os.Open("./test/data/001-helloworld.png")
		image, _ := ioutil.ReadAll(io.LimitReader(file, 1024*1024*50))
		client.SetImageFromBytes(image)
		client.Text()
	}
	client.Close()
}

func BenchmarkClient_Text4(b *testing.B) {
	client := NewClient()
	file, _ := os.Open("./test/data/001-helloworld.png")
	image, _ := ioutil.ReadAll(io.LimitReader(file, 1024*1024*50))
	client.SetImageFromBytes(image)
	for i := 0; i < b.N; i++ {
		client.Text()
	}
	client.Close()
}

func BenchmarkClient_GetBoundingBoxes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		client := NewClient()
		client.SetImage("./test/data/003-longer-text.png")
		client.GetBoundingBoxes(3)
		client.Close()
	}
}

func BenchmarkClient_GetBoundingBoxesVerbose(b *testing.B) {
	for i := 0; i < b.N; i++ {
		client := NewClient()
		client.SetImage("./test/data/003-longer-text.png")
		client.GetBoundingBoxesVerbose()
		client.Close()
	}
}

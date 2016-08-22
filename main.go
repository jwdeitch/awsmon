package main

import (
	"fmt"
	"sync"
	"os/exec"
	"encoding/json"
	"time"
	"net/http"


	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type RegionReport struct {
	EC2Instances []EC2instance
	S3Buckets    []S3Bucket
}

type EC2instance struct {
	Id               string
	Region           string
	State            string
	PublicIpAddress  string
	LaunchTime       time.Time
	PrivateIpAddress string
	InstanceType     string
}

type S3Bucket struct {
	Name            string
	SizeInBytes     int64
	NumberOfObjects int64
	CreateDate      time.Time
}

func main() {

	fmt.Println("Running on http://localhost:9091/")
	http.HandleFunc("/", collectReports) // set router
	err := http.ListenAndServe(":9091", nil) // set listen port
	if err != nil {
		fmt.Println(err.Error())
	}
}

func collectReports(w http.ResponseWriter) {
	regions := []string{"us-west-2", "us-east-1"}
	RegionReports := RegionReport{}

	var wg sync.WaitGroup
	wg.Add(len(regions))

	bucketChan := make(chan int)
	go retrieveBucketReport(bucketChan, &RegionReports)

	for _, region := range regions {
		go retrieveRegionReport(region, &wg, &RegionReports)
	}

	wg.Wait()
	<-bucketChan

	jsonOutput, _ := json.Marshal(RegionReports)

	fmt.Fprintf(w, string(jsonOutput))

}

// Will retrieve region specific details
func retrieveRegionReport(region string, wg *sync.WaitGroup, RegionReport *RegionReport) {

	svc := ec2.New(session.New(), &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeInstances(nil)
	if err != nil {
		panic(err)
	}

	for idx, _ := range resp.Reservations {
		for _, inst := range resp.Reservations[idx].Instances {
			RegionReport.EC2Instances = append(RegionReport.EC2Instances, EC2instance{
				Id: *inst.InstanceId,
				Region: region,
				State: *inst.State.Name,
				PrivateIpAddress: *inst.PrivateIpAddress,
				InstanceType: *inst.InstanceType,
				LaunchTime: *inst.LaunchTime,
				PublicIpAddress: *inst.PublicIpAddress})
		}
	}
	wg.Done()
}

// S3 Buckets are not retrieved by region.
func retrieveBucketReport(wg chan <- int, RegionReport *RegionReport) {
	svc := s3.New(session.New(), &aws.Config{Region: aws.String("us-east-1")})
	bucketList, err := svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		panic(err.Error())
	}
	var bucketWaitList sync.WaitGroup
	bucketWaitList.Add(len(bucketList.Buckets))
	for _, bucket := range bucketList.Buckets {
		go func(bucket *s3.Bucket) {
			defer bucketWaitList.Done()

			// Unfortunately, it doesn't look like the SDK provides a query
			// interface for S3 like the CLI does.
			out, err := exec.Command(
				"aws",
				"s3api",
				"list-objects",
				"--bucket",
				*bucket.Name,
				"--output",
				"json",
				"--query",
				"[sum(Contents[].Size), length(Contents[])]").Output();
			if err == nil {
				var s3Stats []int64
				err = json.Unmarshal(out, &s3Stats)
				if err != nil {
					panic(err.Error())
				}
				RegionReport.S3Buckets = append(RegionReport.S3Buckets, S3Bucket{
					Name: *bucket.Name,
					CreateDate: *bucket.CreationDate,
					SizeInBytes: s3Stats[0],
					NumberOfObjects: s3Stats[1]})
			}
		}(bucket)
	}
	bucketWaitList.Wait()
	wg <- 1
}
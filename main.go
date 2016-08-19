package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws"
	"sync"
	"github.com/aws/aws-sdk-go/service/s3"
	"os/exec"
	"encoding/json"
	"time"
)

type RegionReport struct {
	EC2Instances []EC2instance
	S3Buckets    []S3Bucket
}

type EC2instance struct {
	Id     string
	Region string
}

type S3Bucket struct {
	Name            string
	SizeInBytes     int64
	NumberOfObjects int64
	CreateDate      time.Time
}

func main() {
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

	fmt.Printf("%v", RegionReports)

}

func retrieveRegionReport(region string, wg *sync.WaitGroup, RegionReport *RegionReport) {

	requestChan := make(chan int, 2)

	go func() {
		svc := ec2.New(session.New(), &aws.Config{Region: aws.String(region)})

		resp, err := svc.DescribeInstances(nil)
		if err != nil {
			panic(err)
		}

		for idx, _ := range resp.Reservations {
			for _, inst := range resp.Reservations[idx].Instances {
				RegionReport.EC2Instances = append(RegionReport.EC2Instances, EC2instance{
					Id: *inst.InstanceId,
					Region: region})
			}
		}
		requestChan <- 1
	}()

	<-requestChan
	wg.Done()
}

func retrieveBucketReport(wg chan <- int, RegionReport *RegionReport) {
	svc := s3.New(session.New(), &aws.Config{Region: aws.String("us-east-1")})
	bucketList, err := svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		panic(err.Error())
	}
	var bucketWaitList sync.WaitGroup
	bucketWaitList.Add(len(bucketList.Buckets))
	go func() {
		for _, bucket := range bucketList.Buckets {
			defer bucketWaitList.Done()
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
			if err != nil {
				fmt.Println(err.Error())
			}
			var s3Stats []int64
			fmt.Printf("%v", string(out))
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
	}()
	bucketWaitList.Wait()
	wg<- 1
}
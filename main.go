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
	Region          string
}

func main() {
	regions := []string{"us-west-2", "us-east-1"}
	RegionReports := RegionReport{}

	var wg sync.WaitGroup

	wg.Add(len(regions))

	for _, region := range regions {
		go retrieveRegionReport(region, &wg, &RegionReports)
	}

	wg.Wait()

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

	go func() {
		svc := s3.New(session.New(), &aws.Config{Region: aws.String(region)})
		bucketList, err := svc.ListBuckets(&s3.ListBucketsInput{})
		if err != nil {
			panic(err.Error())
		}
		var bucketStatsWg sync.WaitGroup
		wg.Add(len(bucketList.Buckets))
		go func() {
			for _, bucket := range bucketList.Buckets {
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
				err = json.Unmarshal(out, &s3Stats)
				if err != nil {
					panic(err.Error())
				}
				RegionReport.S3Buckets = append(RegionReport.S3Buckets, S3Bucket{
					Name: *bucket.Name,
					Region: region,
					SizeInBytes: s3Stats[0],
					NumberOfObjects: s3Stats[1]})
			}
			bucketStatsWg.Done()
		}()
		wg.Wait()
		requestChan <- 1
	}()

	<-requestChan
	<-requestChan
	wg.Done()
}
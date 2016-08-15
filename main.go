package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws"
	"sync"
	"github.com/aws/aws-sdk-go/service/s3"
	"os/exec"
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

	s3Chan := make(chan int)

	go func(s3Chan chan <- int, region string) {
		svc := s3.New(session.New(), &aws.Config{Region: aws.String(region)})
		bucketReport, err := svc.ListBuckets(&s3.ListBucketsInput{})
		if err != nil {
			panic(err.Error())
		}
		out, err := exec.Command(
			"aws",
			"s3api",
			"list-objects",
			"--bucket",
			region,
			"--output",
			"json",
			"--query",
			"\"[sum(Contents[].Size), length(Contents[])]\"").Output();
		if err != nil {
			fmt.Println(err.Error())
		}

		fmt.Printf("%v", string(out))
		panic(nil)

		for _, bucket := range bucketReport.Buckets {
			RegionReport.S3Buckets = append(RegionReport.S3Buckets, S3Bucket{
				Name: *bucket.Name,
				Region: region})
		}
		s3Chan <- 1
	}(s3Chan, region)

	<-s3Chan
	wg.Done()
}
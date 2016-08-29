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
	"github.com/aws/aws-sdk-go/service/rds"
)

type RegionReport struct {
	EC2Instances []EC2instance
	S3Buckets    []S3Bucket
	DBInstances  []DBInstance
}

type DBInstance struct {
	Name               string
	Region             string
	InstanceType       string
	State              string
	AllocatedSize      int64
	AutoMinorUpgrade   bool
	MasterUsername     string
	PubliclyAccessible bool
	LaunchTime         time.Time
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

func collectReports(w http.ResponseWriter, r *http.Request) {
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

	////////////////////////
	////////// EC2 /////////
	////////////////////////

	ec2Svc := ec2.New(session.New(), &aws.Config{Region: aws.String(region)})

	ec2Resp, err := ec2Svc.DescribeInstances(nil)
	if err != nil {
		panic(err)
	}

	for ec2Index, _ := range ec2Resp.Reservations {
		for _, ec2Inst := range ec2Resp.Reservations[ec2Index].Instances {
			RegionReport.EC2Instances = append(RegionReport.EC2Instances, EC2instance{
				Id: *ec2Inst.InstanceId,
				Region: region,
				State: *ec2Inst.State.Name,
				PrivateIpAddress: *ec2Inst.PrivateIpAddress,
				InstanceType: *ec2Inst.InstanceType,
				LaunchTime: *ec2Inst.LaunchTime,
				PublicIpAddress: *ec2Inst.PublicIpAddress})
		}
	}


	////////////////////////
	////////// RDS /////////
	////////////////////////

	rdsSvc := rds.New(session.New(), &aws.Config{Region: aws.String(region)})

	rdsResp, err := rdsSvc.DescribeDBInstances(nil)
	if err != nil {
		panic(err)
	}

	for _, rdsIndex := range rdsResp.DBInstances {
		RegionReport.DBInstances = append(RegionReport.DBInstances, DBInstance{
			Name: *rdsIndex.DBInstanceIdentifier,
			Region: region,
			State: *rdsIndex.DBInstanceStatus,
			InstanceType: *rdsIndex.DBInstanceClass,
			AllocatedSize: *rdsIndex.AllocatedStorage,
			LaunchTime: *rdsIndex.InstanceCreateTime,
			MasterUsername: *rdsIndex.MasterUsername,
			PubliclyAccessible: *rdsIndex.PubliclyAccessible,
			AutoMinorUpgrade: *rdsIndex.AutoMinorVersionUpgrade})
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
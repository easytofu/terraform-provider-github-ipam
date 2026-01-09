# Example: Allocate a VPC CIDR from a pool (Mode 1: pool_id)
resource "gitipam_allocation" "vpc_primary" {
  pool_id   = "production-us-east"
  cidr_mask = 16
  name      = "vpc-primary"

  metadata = {
    purpose     = "vpc"
    environment = "production"
  }
}

# Example: Allocate subnets within the VPC (Mode 2: parent_cidr)
resource "gitipam_allocation" "subnet_public_az1" {
  parent_cidr = gitipam_allocation.vpc_primary.cidr
  cidr_mask   = 24
  name        = "public-az1"

  metadata = {
    tier = "public"
    az   = "us-east-1a"
  }
}

resource "gitipam_allocation" "subnet_public_az2" {
  parent_cidr = gitipam_allocation.vpc_primary.cidr
  cidr_mask   = 24
  name        = "public-az2"

  metadata = {
    tier = "public"
    az   = "us-east-1b"
  }
}

resource "gitipam_allocation" "subnet_private_az1" {
  parent_cidr = gitipam_allocation.vpc_primary.cidr
  cidr_mask   = 20
  name        = "private-az1"

  metadata = {
    tier = "private"
    az   = "us-east-1a"
  }
}

# Use the allocated CIDRs with AWS resources
resource "aws_vpc" "main" {
  cidr_block           = gitipam_allocation.vpc_primary.cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "main-vpc"
  }
}

resource "aws_subnet" "public_az1" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = gitipam_allocation.subnet_public_az1.cidr
  availability_zone = "us-east-1a"

  tags = {
    Name = "public-az1"
    Tier = "public"
  }
}

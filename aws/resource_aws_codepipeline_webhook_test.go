package aws

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccAWSCodePipelineWebhook_basic(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("Environment variable GITHUB_TOKEN is not set")
	}

	name := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAWSCodePipelineDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAWSCodePipelineWebhookConfig_basic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSCodePipelineExists("aws_codepipeline.bar"),
					testAccCheckAWSCodePipelineWebhookExists("aws_codepipeline_webhook.bar"),
					resource.TestCheckResourceAttrSet("aws_codepipeline_webhook.bar", "id"),
					resource.TestCheckResourceAttrSet("aws_codepipeline_webhook.bar", "url"),
				),
			},
		},
	})
}

func testAccCheckAWSCodePipelineWebhookExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No webhook ARN is set as ID")
		}

		conn := testAccProvider.Meta().(*AWSClient).codepipelineconn

		webhooks, err := getAllCodePipelineWebhooks(conn)
		if err != nil {
			return err
		}

		var arn string
		for _, hook := range webhooks {
			arn = *hook.Arn
			if rs.Primary.ID == arn {
				return nil
			}
		}

		return fmt.Errorf("Webhook %s not found", rs.Primary.ID)
	}
}

func testAccAWSCodePipelineWebhookConfig_basic(rName string) string {
	return fmt.Sprintf(`
resource "aws_s3_bucket" "foo" {
  bucket = "tf-test-pipeline-%s"
  acl    = "private"
}

resource "aws_iam_role" "codepipeline_role" {
  name = "codepipeline-role-%s"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "codepipeline.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOF
}

resource "aws_iam_role_policy" "codepipeline_policy" {
  name = "codepipeline_policy"
  role = "${aws_iam_role.codepipeline_role.id}"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect":"Allow",
      "Action": [
        "s3:GetObject",
        "s3:GetObjectVersion",
        "s3:GetBucketVersioning"
      ],
      "Resource": [
        "${aws_s3_bucket.foo.arn}",
        "${aws_s3_bucket.foo.arn}/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "codebuild:BatchGetBuilds",
        "codebuild:StartBuild"
      ],
      "Resource": "*"
    }
  ]
}
EOF
}

resource "aws_codepipeline" "bar" {
  name     = "test-pipeline-%s"
  role_arn = "${aws_iam_role.codepipeline_role.arn}"

  artifact_store {
    location = "${aws_s3_bucket.foo.bucket}"
    type     = "S3"

    encryption_key {
      id   = "1234"
      type = "KMS"
    }
  }

  stage {
    name = "Source"

    action {
      name             = "Source"
      category         = "Source"
      owner            = "ThirdParty"
      provider         = "GitHub"
      version          = "1"
      output_artifacts = ["test"]

      configuration {
        Owner  = "lifesum-terraform"
        Repo   = "test"
        Branch = "master"
      }
    }
  }

  stage {
    name = "Build"

    action {
      name            = "Build"
      category        = "Build"
      owner           = "AWS"
      provider        = "CodeBuild"
      input_artifacts = ["test"]
      version         = "1"

      configuration {
        ProjectName = "test"
      }
    }
  }
}

resource "aws_codepipeline_webhook" "bar" {
    name = "test-webhook-%s" 

    auth {
      type         = "GITHUB_HMAC" 
      secret_token = "super-secret"
    }

    filter {
      json_path    = "$.ref"
      match_equals = "refs/head/{Branch}"
    }

    target {
        action   = "Source"
        pipeline = "${aws_codepipeline.bar.name}"
    }    
}
`, rName, rName, rName, rName)
}

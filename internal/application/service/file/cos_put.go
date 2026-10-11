package file

import (
	"context"
	"fmt"
	"io"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/utils"
)

// PutCOSObject uploads reader to the COS object named by objectKey using the
// given bucket/region/credentials. It is the migration primitive used when
// moving local files to the COS backend: unlike SaveFile/SaveBytes the object
// key is caller-controlled so the original local layout can be preserved.
func PutCOSObject(ctx context.Context, bucketName, region, secretID, secretKey, objectKey string, reader io.Reader) error {
	if err := utils.SafeObjectKey(objectKey); err != nil {
		return fmt.Errorf("invalid object key: %w", err)
	}
	svc, err := newCosClient(bucketName, region, secretID, secretKey)
	if err != nil {
		return err
	}
	ctx, cancel := objectStorageTransferContext(ctx)
	defer cancel()
	if _, err := svc.client.Object.Put(ctx, objectKey, reader, nil); err != nil {
		return fmt.Errorf("failed to upload object to COS: %w", err)
	}
	logger.Infof(ctx, "PutCOSObject: uploaded %s/%s", bucketName, objectKey)
	return nil
}

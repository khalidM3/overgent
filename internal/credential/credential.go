package credential

import "context"

const serviceName = "dev.stickguy.device"

func Put(ctx context.Context, account, secret string) error   { return put(ctx, account, secret) }
func Get(ctx context.Context, account string) (string, error) { return get(ctx, account) }
func Delete(ctx context.Context, account string) error        { return remove(ctx, account) }

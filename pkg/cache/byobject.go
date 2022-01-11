package cache

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type ByObjectOptions struct {
	NewObjectCaches map[client.Object]NewCacheFunc
	NewDefaultCache NewCacheFunc
}

func BuilderByObject(byObjectOpts ByObjectOptions) NewCacheFunc {
	return func(config *rest.Config, opts Options) (Cache, error) {
		opts, err := defaultOpts(config, opts)
		if err != nil {
			return nil, err
		}

		if byObjectOpts.NewDefaultCache == nil {
			byObjectOpts.NewDefaultCache = New
		}
		defaultCache, err := byObjectOpts.NewDefaultCache(config, opts)
		if err != nil {
			return nil, err
		}

		gvkToCache := map[schema.GroupVersionKind]Cache{}
		for obj, newCacheFunc := range byObjectOpts.NewObjectCaches {
			gvk, err := apiutil.GVKForObject(obj, opts.Scheme)
			if err != nil {
				return nil, err
			}
			gvkToCache[gvk], err = newCacheFunc(config, opts)
			if err != nil {
				return nil, err
			}
		}

		return &byObject{
			gvkToCache:   gvkToCache,
			defaultCache: defaultCache,
			scheme:       opts.Scheme,
		}, nil
	}
}

type byObject struct {
	gvkToCache   map[schema.GroupVersionKind]Cache
	defaultCache Cache
	scheme       *runtime.Scheme
}

func (c *byObject) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	cache, err := c.cacheForObject(obj)
	if err != nil {
		return err
	}
	return cache.Get(ctx, key, obj)
}

func (c *byObject) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := apiutil.GVKForObject(list, c.scheme)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(gvk.Kind, "List") {
		return fmt.Errorf("non-list type %T (kind %q) passed as output", list, gvk)
	}
	gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	return c.cacheForGVK(gvk).List(ctx, list, opts...)
}

func (c *byObject) GetInformer(ctx context.Context, obj client.Object) (Informer, error) {
	cache, err := c.cacheForObject(obj)
	if err != nil {
		return nil, err
	}
	return cache.GetInformer(ctx, obj)
}

func (c *byObject) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (Informer, error) {
	return c.cacheForGVK(gvk).GetInformerForKind(ctx, gvk)
}

func (c *byObject) Start(ctx context.Context) error {
	for gvk, cache := range c.gvkToCache {
		go func(gvk schema.GroupVersionKind, cache Cache) {
			err := cache.Start(ctx)
			if err != nil {
				log.Error(err, "gvk cache failed to start", "gvk", gvk)
			}
		}(gvk, cache)
	}
	go func() {
		err := c.defaultCache.Start(ctx)
		if err != nil {
			log.Error(err, "default cache failed to start")
		}
	}()
	<-ctx.Done()
	return nil
}

func (c *byObject) WaitForCacheSync(ctx context.Context) bool {
	synced := true
	for _, cache := range c.gvkToCache {
		if s := cache.WaitForCacheSync(ctx); !s {
			synced = synced && s
		}
	}
	return synced && c.defaultCache.WaitForCacheSync(ctx)
}

func (c *byObject) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	cache, err := c.cacheForObject(obj)
	if err != nil {
		return err
	}
	return cache.IndexField(ctx, obj, field, extractValue)
}

func (c *byObject) cacheForObject(obj client.Object) (Cache, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}
	return c.cacheForGVK(gvk), nil
}

func (c *byObject) cacheForGVK(gvk schema.GroupVersionKind) Cache {
	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache
	}
	return c.defaultCache
}

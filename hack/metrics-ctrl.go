package main

func main() {

	//// initalize new manager
	//mgr, err := cr.NewManager(cr.GetConfigOrDie(), cr.Options{})
	//if err != nil {
	//	log.Fatal("failed to start controller runtime manager:", err)
	//}
	//
	//err = metricsV1beta1.AddToScheme(mgr.GetScheme())
	//if err != nil {
	//	log.Fatal("failed to add scheme:", err)
	//}
	//
	//if err := cr.NewControllerManagedBy(mgr).
	//	For( &metricsV1beta1.NodeMetrics{}).
	//	Complete(&recon{mgr.GetClient()}); err != nil {
	//		log.Fatalf("failed to setup controller: %s", err)
	//}

	// add watcher for type and use a default enqueing handler
	//
	//if err := ctrl.Watch(&source.Kind{Type: &metricsV1beta1.NodeMetrics{}}, &handler.EnqueueRequestForObject{}); err != nil {
	//	log.Fatalf("failed to watch Pod: %s", err)
	//}

	// <<<<< End Use Controller Directly >>>>>

	// Start manager to start internal controller
	//if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
	//	log.Fatal("unable to run manager:", err)
	//}
}

//type recon struct{
//	client.Client
//}
//
//func (r recon) Reconcile(req reconcile.Request) (reconcile.Result, error) {
//	ctx := context.TODO()
//
//	var metrics metricsV1beta1.PodMetrics
//	if err := r.Get(ctx, req.NamespacedName, &metrics); err != nil {
//		return reconcile.Result{Requeue: true}, nil
//	}
//	fmt.Printf("Got metrics %v\n", metrics)
//	return cr.Result{Requeue: false}, nil
//}

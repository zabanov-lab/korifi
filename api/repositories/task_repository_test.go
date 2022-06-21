package repositories_test

import (
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TaskRepository", func() {
	var (
		taskRepo *repositories.TaskRepo
		org      *korifiv1alpha1.CFOrg
		space    *korifiv1alpha1.CFSpace
		cfApp    *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		taskRepo = repositories.NewTaskRepo(userClientFactory, 2*time.Second)

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		cfApp = createApp(space.Name)
	})

	Describe("CreateTask", func() {
		var (
			createMessage       repositories.CreateTaskMessage
			taskRecord          repositories.TaskRecord
			createErr           error
			dummyTaskController func(*korifiv1alpha1.CFTask) error
			killController      chan bool
			controllerSync      *sync.WaitGroup
		)

		BeforeEach(func() {
			dummyTaskController = func(cft *korifiv1alpha1.CFTask) error {
				meta.SetStatusCondition(&cft.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.TaskInitializedConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "foo",
					Message: "bar",
				})
				cft.Status.SequenceID = 6
				cft.Status.MemoryMB = 256
				cft.Status.DiskQuotaMB = 128
				cft.Status.DropletRef.Name = cfApp.Spec.CurrentDropletRef.Name
				return k8sClient.Status().Update(ctx, cft)
			}
			controllerSync = &sync.WaitGroup{}
			controllerSync.Add(1)
			killController = make(chan bool)
			createMessage = repositories.CreateTaskMessage{
				Command:   "  echo    hello  ",
				SpaceGUID: space.Name,
				AppGUID:   cfApp.Name,
			}
		})

		JustBeforeEach(func() {
			tasksWatch, err := k8sClient.Watch(
				ctx,
				&korifiv1alpha1.CFTaskList{},
				client.InNamespace(space.Name),
			)
			Expect(err).NotTo(HaveOccurred())

			defer tasksWatch.Stop()
			watchChan := tasksWatch.ResultChan()

			go func() {
				defer GinkgoRecover()
				defer controllerSync.Done()

				timer := time.NewTimer(2 * time.Second)
				defer timer.Stop()

				for {
					select {
					case e := <-watchChan:
						cft, ok := e.Object.(*korifiv1alpha1.CFTask)
						if !ok {
							time.Sleep(100 * time.Millisecond)
							continue
						}

						Expect(dummyTaskController(cft)).To(Succeed())
						return

					case <-timer.C:
						return

					case <-killController:
						return
					}
				}
			}()

			taskRecord, createErr = taskRepo.CreateTask(ctx, authInfo, createMessage)
		})

		AfterEach(func() {
			close(killController)
			controllerSync.Wait()
		})

		It("returns forbidden error", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can create tasks", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates the task", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(taskRecord.Name).NotTo(BeEmpty())
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo hello"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).NotTo(BeZero())
				Expect(taskRecord.CreationTimestamp).To(BeTemporally("~", time.Now(), 5*time.Second))
				Expect(taskRecord.MemoryMB).To(BeNumerically("==", 256))
				Expect(taskRecord.DiskMB).To(BeNumerically("==", 128))
				Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
			})

			When("the task never becomes initialized", func() {
				BeforeEach(func() {
					dummyTaskController = func(cft *korifiv1alpha1.CFTask) error {
						return nil
					}
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("did not get initialized")))
				})
			})
		})

		When("unprivileged client creation fails", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("failed to build user client")))
			})
		})
	})
})
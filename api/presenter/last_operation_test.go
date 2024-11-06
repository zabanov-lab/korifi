package presenter_test

import (
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/presenter/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ForLastOperation", func() {
	var (
		record   *fake.RecordWithLastOperation
		response presenter.LastOperationResponse
	)

	BeforeEach(func() {
		record = new(fake.RecordWithLastOperation)
		record.GetCreatedAtReturns(time.UnixMilli(1000))
		record.GetStateReturns(repositories.RecordState{Value: model.CFResourceStateUnknown})
	})

	JustBeforeEach(func() {
		response = presenter.ForLastOperation(record)
	})

	It("returns created at", func() {
		Expect(response.CreatedAt).To(RepresentsTime(time.UnixMilli(1000)))
	})

	It("returns empty updatedAt", func() {
		Expect(response.UpdatedAt).To(BeEmpty())
	})

	When("the record has been updated", func() {
		BeforeEach(func() {
			record.GetUpdatedAtReturns(tools.PtrTo(time.UnixMilli(2000)))
		})

		It("returns updatedAt", func() {
			Expect(response.UpdatedAt).To(RepresentsTime(time.UnixMilli(2000)))
		})
	})

	Describe("type", func() {
		It("returns create", func() {
			Expect(response.Type).To(Equal("create"))
		})

		When("the record has been updated", func() {
			BeforeEach(func() {
				record.GetUpdatedAtReturns(tools.PtrTo(time.UnixMilli(2000)))
			})

			It("returns update", func() {
				Expect(response.Type).To(Equal("update"))
			})
		})

		When("the record is deleted", func() {
			BeforeEach(func() {
				record.GetUpdatedAtReturns(tools.PtrTo(time.UnixMilli(2000)))
				record.GetDeletedAtReturns(tools.PtrTo(time.UnixMilli(3000)))
			})

			It("returns delete", func() {
				Expect(response.Type).To(Equal("delete"))
			})
		})
	})

	Describe("state", func() {
		It("returns in progress state", func() {
			Expect(response.State).To(Equal("in progress"))
		})

		When("ready", func() {
			BeforeEach(func() {
				record.GetStateReturns(repositories.RecordState{Value: model.CFResourceStateReady})
			})

			It("returns succeeded state", func() {
				Expect(response.State).To(Equal("succeeded"))
			})
		})

		When("failed", func() {
			BeforeEach(func() {
				record.GetStateReturns(repositories.RecordState{Value: model.CFResourceStateFailed, Description: "it failed"})
			})

			It("returns failed state", func() {
				Expect(response.State).To(Equal("failed"))
				Expect(response.Description).To(Equal("it failed"))
			})
		})
	})
})
